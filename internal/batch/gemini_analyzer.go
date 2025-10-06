package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/database"
	"github.com/naoya0117/shuron2025/api/internal/ratelimit"
)

const (
	AnalyzeKind = "analyze"
)

// GeminiAnalyzer manages repository analysis with rate limit handling
type GeminiAnalyzer struct {
	client     *GeminiClient
	db         *database.DB
	queue      *ratelimit.FailedQueue
	gitManager *GitManager
	workDir    string
}

func NewGeminiAnalyzer(client *GeminiClient, db *database.DB, gitManager *GitManager, workDir string) *GeminiAnalyzer {
	return &GeminiAnalyzer{
		client:     client,
		db:         db,
		queue:      ratelimit.NewFailedQueue(db),
		gitManager: gitManager,
		workDir:    workDir,
	}
}

type AnalysisRequest struct {
	RepositoryID   int    `json:"repositoryId"`
	RepositoryPath string `json:"repositoryPath"`
	CheckQueryID   int    `json:"checkQueryId"`
}

// StartAnalysisWithRateLimitHandling begins analysis with automatic rate limit handling
func (ga *GeminiAnalyzer) StartAnalysisWithRateLimitHandling(ctx context.Context) error {
	// Check if we're in a rate limit sleep state
	if err := ga.queue.WaitUntilResumeTime(ctx, AnalyzeKind); err != nil {
		return fmt.Errorf("failed to wait for rate limit resume: %w", err)
	}

	// Process any failed items first
	if err := ga.processFailedQueue(ctx); err != nil {
		return fmt.Errorf("failed to process failed queue: %w", err)
	}

	log.Printf("Rate limit handling ready for analysis")
	return nil
}

// processFailedQueue retries failed analysis requests
func (ga *GeminiAnalyzer) processFailedQueue(ctx context.Context) error {
	for {
		items, err := ga.queue.DequeueBatch(ctx, AnalyzeKind, 10)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			log.Printf("No failed analysis items to retry")
			break
		}

		log.Printf("Retrying %d failed analysis requests", len(items))

		for _, item := range items {
			var req AnalysisRequest
			if err := json.Unmarshal([]byte(item.Payload), &req); err != nil {
				log.Printf("Failed to unmarshal analysis payload for item %d: %v", item.ID, err)
				// Delete malformed items
				_ = ga.queue.DeleteItem(ctx, item.ID)
				continue
			}

			// Get repository info
			repo, err := ga.db.GetRepository(req.RepositoryID)
			if err != nil {
				log.Printf("Failed to get repository %d: %v", req.RepositoryID, err)
				_ = ga.queue.DeleteItem(ctx, item.ID)
				continue
			}

			// Get the check query
			queries, err := ga.db.GetCheckQueries()
			if err != nil {
				log.Printf("Failed to get check queries: %v", err)
				continue
			}

			var query *database.CheckQuery
			for _, q := range queries {
				if q.ID == req.CheckQueryID {
					query = &q
					break
				}
			}

			if query == nil {
				log.Printf("Check query %d not found, deleting item", req.CheckQueryID)
				_ = ga.queue.DeleteItem(ctx, item.ID)
				continue
			}

			// Clone repository for retry
			repoDir := fmt.Sprintf("%s/%d", ga.workDir, req.RepositoryID)

			// Clean up any existing directory first
			if err := ga.gitManager.Cleanup(repoDir); err != nil {
				log.Printf("Warning: failed to cleanup existing directory %s: %v", repoDir, err)
			}

			// Clone the repository
			if err := ga.gitManager.Clone(repo.URL, repoDir); err != nil {
				log.Printf("Failed to clone repository %s for retry: %v", repo.URL, err)
				// Keep in queue for later retry
				continue
			}

			// Ensure cleanup after retry
			defer func(dir string) {
				if err := ga.gitManager.Cleanup(dir); err != nil {
					log.Printf("Warning: failed to cleanup %s after retry: %v", dir, err)
				}
			}(repoDir)

			// Retry the analysis
			response, err := ga.client.AnalyzeRepositoryWithRetry(repoDir, *query, 3)
			if err != nil {
				if IsGeminiRateLimitError(err) {
					log.Printf("Rate limit hit while retrying, re-entering sleep mode")
					return ga.handleRateLimit(ctx, err, &req)
				}

				log.Printf("Failed to retry analysis for repo %d: %v", req.RepositoryID, err)
				// Keep in queue for later retry
				continue
			}

			log.Printf("Successfully retried analysis for repository %d, query %d", req.RepositoryID, req.CheckQueryID)

			// Update database with result
			if err := ga.db.InsertEasyCheckedRepository(req.RepositoryID, req.CheckQueryID, response, "completed"); err != nil {
				log.Printf("Warning: failed to save retried analysis result: %v", err)
			}

			// Delete successfully processed item
			if err := ga.queue.DeleteItem(ctx, item.ID); err != nil {
				log.Printf("Warning: failed to delete processed item %d: %v", item.ID, err)
			}
		}
	}

	return nil
}

// AnalyzeWithRateLimitHandling wraps AnalyzeRepository with rate limit handling
func (ga *GeminiAnalyzer) AnalyzeWithRateLimitHandling(ctx context.Context, repoID int, repoPath string, query database.CheckQuery) (string, error) {
	response, err := ga.client.AnalyzeRepositoryWithRetry(repoPath, query, 3)
	if err != nil {
		if IsGeminiRateLimitError(err) {
			req := &AnalysisRequest{
				RepositoryID:   repoID,
				RepositoryPath: repoPath,
				CheckQueryID:   query.ID,
			}
			return "", ga.handleRateLimit(ctx, err, req)
		}
		return "", err
	}

	return response, nil
}

// handleRateLimit processes rate limit errors and sets up sleep state
func (ga *GeminiAnalyzer) handleRateLimit(ctx context.Context, err error, req *AnalysisRequest) error {
	var rlErr *GeminiRateLimitError
	if !IsGeminiRateLimitError(err) {
		return err
	}

	// Type assertion to extract reset time
	if e, ok := err.(*GeminiRateLimitError); ok {
		rlErr = e
	} else {
		rlErr = &GeminiRateLimitError{message: "Gemini rate limit exceeded"}
	}

	// Enqueue the failed request
	if req != nil {
		if err := ga.queue.Enqueue(ctx, AnalyzeKind, req, "rate-limit"); err != nil {
			log.Printf("Warning: failed to enqueue failed analysis request: %v", err)
		}
	}

	// Calculate resume time
	now := time.Now()
	resumeAt := ratelimit.ChooseResumeAt(now, rlErr.resetTime)

	log.Printf("Rate limit detected for Gemini analysis. Reason: %s", rlErr.message)
	log.Printf("Setting sleep state until %s", resumeAt.Format("2006-01-02 15:04:05 MST"))

	// Set sleeping state
	if err := ga.queue.SetState(ctx, AnalyzeKind, true, &resumeAt, "gemini"); err != nil {
		return fmt.Errorf("failed to set rate limit state: %w", err)
	}

	// Actually sleep until resume time
	if err := ratelimit.SleepUntilCtx(ctx, resumeAt); err != nil {
		return fmt.Errorf("sleep interrupted: %w", err)
	}

	// Clear sleeping state
	if err := ga.queue.SetState(ctx, AnalyzeKind, false, nil, ""); err != nil {
		return fmt.Errorf("failed to clear rate limit state: %w", err)
	}

	log.Printf("Rate limit period ended for Gemini analysis, resuming operations")

	// Return nil to indicate successful handling - failed items are already enqueued
	// and will be retried in the next processFailedQueue call
	return nil
}

// GetRateLimitStatus returns current rate limit state
func (ga *GeminiAnalyzer) GetRateLimitStatus(ctx context.Context) (*database.RateLimitState, error) {
	return ga.queue.GetState(ctx, AnalyzeKind)
}
