package github

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
	CollectKind = "collect"
)

// Collector manages GitHub repository collection with rate limit handling
type Collector struct {
	client *Client
	db     *database.DB
	queue  *ratelimit.FailedQueue
}

func NewCollector(client *Client, db *database.DB) *Collector {
	return &Collector{
		client: client,
		db:     db,
		queue:  ratelimit.NewFailedQueue(db),
	}
}

type RepositoryRequest struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// CollectWithRateLimitHandling collects repositories with automatic rate limit handling
func (c *Collector) CollectWithRateLimitHandling(ctx context.Context, sessionID string) error {
	// Check if we're in a rate limit sleep state
	if err := c.queue.WaitUntilResumeTime(ctx, CollectKind); err != nil {
		return fmt.Errorf("failed to wait for rate limit resume: %w", err)
	}

	// Process any failed items first
	if err := c.processFailedQueue(ctx); err != nil {
		return fmt.Errorf("failed to process failed queue: %w", err)
	}

	log.Printf("Rate limit handling ready for session %s", sessionID)
	return nil
}

// processFailedQueue retries failed repository requests
func (c *Collector) processFailedQueue(ctx context.Context) error {
	for {
		items, err := c.queue.DequeueBatch(ctx, CollectKind, 20)
		if err != nil {
			return err
		}

		if len(items) == 0 {
			log.Printf("No failed items to retry for kind %s", CollectKind)
			break
		}

		log.Printf("Retrying %d failed collection requests", len(items))

		for _, item := range items {
			var req RepositoryRequest
			if err := json.Unmarshal([]byte(item.Payload), &req); err != nil {
				log.Printf("Failed to unmarshal payload for item %d: %v", item.ID, err)
				// Delete malformed items
				_ = c.queue.DeleteItem(ctx, item.ID)
				continue
			}

			// Retry the request
			hasDockerfile, err := c.client.HasDockerfile(ctx, req.Owner, req.Name)
			if err != nil {
				if IsRateLimitError(err) {
					log.Printf("Rate limit hit while retrying, re-entering sleep mode")
					return c.handleRateLimit(ctx, err, &req)
				}

				log.Printf("Failed to retry repository %s/%s: %v", req.Owner, req.Name, err)
				// Keep in queue for later retry or mark as permanently failed
				continue
			}

			log.Printf("Successfully retried repository %s/%s (hasDockerfile=%v)", req.Owner, req.Name, hasDockerfile)

			// Delete successfully processed item
			if err := c.queue.DeleteItem(ctx, item.ID); err != nil {
				log.Printf("Warning: failed to delete processed item %d: %v", item.ID, err)
			}
		}
	}

	return nil
}

// HandleDockerfileCheck wraps HasDockerfile with rate limit handling
func (c *Collector) HandleDockerfileCheck(ctx context.Context, owner, name string) (bool, error) {
	hasDockerfile, err := c.client.HasDockerfile(ctx, owner, name)
	if err != nil {
		if IsRateLimitError(err) {
			req := &RepositoryRequest{Owner: owner, Name: name}
			return false, c.handleRateLimit(ctx, err, req)
		}
		return false, err
	}

	return hasDockerfile, nil
}

// handleRateLimit processes rate limit errors and sets up sleep state
func (c *Collector) handleRateLimit(ctx context.Context, err error, req *RepositoryRequest) error {
	var rlErr *RateLimitError
	if !IsRateLimitError(err) {
		return err
	}

	// Type assertion to extract reset time
	if e, ok := err.(*RateLimitError); ok {
		rlErr = e
	} else {
		rlErr = &RateLimitError{Message: "Rate limit exceeded"}
	}

	// Enqueue the failed request
	if req != nil {
		if err := c.queue.Enqueue(ctx, CollectKind, req, "rate-limit"); err != nil {
			log.Printf("Warning: failed to enqueue failed request: %v", err)
		}
	}

	// Calculate resume time
	now := time.Now()
	resumeAt := ratelimit.ChooseResumeAt(now, rlErr.ResetTime)

	log.Printf("Rate limit detected for GitHub collection. Reason: %s", rlErr.Message)
	log.Printf("Setting sleep state until %s", resumeAt.Format("2006-01-02 15:04:05 MST"))

	// Set sleeping state
	if err := c.queue.SetState(ctx, CollectKind, true, &resumeAt, "github"); err != nil {
		return fmt.Errorf("failed to set rate limit state: %w", err)
	}

	// Actually sleep until resume time
	if err := ratelimit.SleepUntilCtx(ctx, resumeAt); err != nil {
		return fmt.Errorf("sleep interrupted: %w", err)
	}

	// Clear sleeping state
	if err := c.queue.SetState(ctx, CollectKind, false, nil, ""); err != nil {
		return fmt.Errorf("failed to clear rate limit state: %w", err)
	}

	log.Printf("Rate limit period ended for GitHub collection, resuming operations")

	// Retry the failed request immediately after waking up
	if req != nil {
		hasDockerfile, retryErr := c.client.HasDockerfile(ctx, req.Owner, req.Name)
		if retryErr != nil {
			// If still rate limited, will be handled by next call
			return retryErr
		}
		log.Printf("Successfully retried %s/%s after rate limit (hasDockerfile=%v)", req.Owner, req.Name, hasDockerfile)
	}

	return nil
}

// GetRateLimitStatus returns current rate limit state
func (c *Collector) GetRateLimitStatus(ctx context.Context) (*database.RateLimitState, error) {
	return c.queue.GetState(ctx, CollectKind)
}
