package batch

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/database"
)

type RepositoryProcessor struct {
	db                   *database.DB
	geminiClient         *GeminiClient
	gitManager           *GitManager
	workDir              string
	maxRetries           int
	rateLimitScheduler   *RateLimitScheduler
}

func NewRepositoryProcessor(db *database.DB, geminiClient *GeminiClient, gitManager *GitManager, workDir string) *RepositoryProcessor {
	return &RepositoryProcessor{
		db:                 db,
		geminiClient:       geminiClient,
		gitManager:         gitManager,
		workDir:            workDir,
		maxRetries:         3,
		rateLimitScheduler: NewRateLimitScheduler(),
	}
}

func (rp *RepositoryProcessor) ProcessRepository(repo database.Repository) error {
	repoDir := filepath.Join(rp.workDir, fmt.Sprintf("%d", repo.ID))
	
	defer func() {
		if err := rp.gitManager.Cleanup(repoDir); err != nil {
			log.Printf("Failed to cleanup %s: %v", repoDir, err)
		}
	}()
	
	log.Printf("Processing repository: %s (%s)", repo.NameWithOwner, repo.URL)
	
	if err := rp.gitManager.Clone(repo.URL, repoDir); err != nil {
		return fmt.Errorf("failed to clone repository %s: %w", repo.URL, err)
	}
	
	if err := rp.gitManager.ValidateRepository(repoDir); err != nil {
		return fmt.Errorf("repository validation failed for %s: %w", repoDir, err)
	}
	
	size, err := rp.gitManager.GetRepositorySize(repoDir)
	if err != nil {
		log.Printf("Warning: failed to get repository size for %s: %v", repo.NameWithOwner, err)
	} else {
		log.Printf("Repository %s size: %.2f MB", repo.NameWithOwner, float64(size)/(1024*1024))
		
		const maxSizeBytes = 100 * 1024 * 1024
		if size > maxSizeBytes {
			log.Printf("Warning: Repository %s is large (%.2f MB), analysis may be slow", 
				repo.NameWithOwner, float64(size)/(1024*1024))
		}
	}
	
	queries, err := rp.db.GetCheckQueries()
	if err != nil {
		return fmt.Errorf("failed to get check queries: %w", err)
	}
	
	if len(queries) == 0 {
		log.Printf("No check queries found, skipping repository %s", repo.NameWithOwner)
		return nil
	}
	
	log.Printf("Found %d check queries to execute for repository %s", len(queries), repo.NameWithOwner)
	
	successCount := 0
	for _, query := range queries {
		if err := rp.executeCheck(repo, query, repoDir); err != nil {
			log.Printf("Check failed for repo %d (%s), query %d (%s): %v", 
				repo.ID, repo.NameWithOwner, query.ID, query.Name, err)
			continue
		}
		successCount++
	}
	
	log.Printf("Completed repository %s: %d/%d checks succeeded", 
		repo.NameWithOwner, successCount, len(queries))
	
	return nil
}

func (rp *RepositoryProcessor) executeCheck(repo database.Repository, query database.CheckQuery, repoDir string) error {
	existing, err := rp.db.GetEasyCheckedRepository(repo.ID, query.ID)
	if err != nil {
		return fmt.Errorf("failed to check existing result: %w", err)
	}
	
	if existing != nil && existing.Status == "completed" {
		log.Printf("Check already completed for repo %d, query %d, skipping", repo.ID, query.ID)
		return nil
	}
	
	log.Printf("Executing check: %s for repository %s", query.Name, repo.NameWithOwner)
	
	if err := rp.db.InsertEasyCheckedRepository(repo.ID, query.ID, "", "pending"); err != nil {
		return fmt.Errorf("failed to create pending check record: %w", err)
	}
	
	startTime := time.Now()
	response, err := rp.geminiClient.AnalyzeRepositoryWithRetry(repoDir, query, rp.maxRetries)
	duration := time.Since(startTime)
	
	if err != nil {
		if rateLimitErr, isRateLimit := err.(*RateLimitError); isRateLimit {
			log.Printf("Rate limit encountered for repo %s, query %s: %v", 
				repo.NameWithOwner, query.Name, rateLimitErr)
			
			if err := rp.db.InsertEasyCheckedRepository(repo.ID, query.ID, 
				"Rate limit reached - will retry after reset", "rate_limited"); err != nil {
				log.Printf("Failed to update check status to rate_limited: %v", err)
			}
			
			return &RateLimitError{message: "rate limit reached during analysis"}
		}
		
		log.Printf("Gemini analysis failed for repo %s, query %s (took %v): %v", 
			repo.NameWithOwner, query.Name, duration, err)
		
		if updateErr := rp.db.InsertEasyCheckedRepository(repo.ID, query.ID, 
			fmt.Sprintf("Error: %v", err), "failed"); updateErr != nil {
			log.Printf("Failed to update check status to failed: %v", updateErr)
		}
		return err
	}
	
	log.Printf("Gemini analysis completed for repo %s, query %s (took %v)", 
		repo.NameWithOwner, query.Name, duration)
	
	if err := rp.db.InsertEasyCheckedRepository(repo.ID, query.ID, response, "completed"); err != nil {
		return fmt.Errorf("failed to save check result: %w", err)
	}
	
	return nil
}

func (rp *RepositoryProcessor) GetProcessingStats(repositories []database.Repository) (*ProcessingStats, error) {
	stats := &ProcessingStats{
		TotalRepositories: len(repositories),
	}
	
	for _, repo := range repositories {
		queries, err := rp.db.GetCheckQueries()
		if err != nil {
			return nil, fmt.Errorf("failed to get check queries: %w", err)
		}
		
		repoCompleted := true
		for _, query := range queries {
			existing, err := rp.db.GetEasyCheckedRepository(repo.ID, query.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to check existing result: %w", err)
			}
			
			if existing == nil || existing.Status != "completed" {
				repoCompleted = false
				break
			}
		}
		
		if repoCompleted {
			stats.CompletedRepositories++
		}
	}
	
	return stats, nil
}

func (rp *RepositoryProcessor) ValidateEnvironment() error {
	if err := rp.geminiClient.CheckAuthentication(); err != nil {
		return fmt.Errorf("gemini CLI authentication check failed: %w", err)
	}
	
	queries, err := rp.db.GetCheckQueries()
	if err != nil {
		return fmt.Errorf("failed to get check queries: %w", err)
	}
	
	if len(queries) == 0 {
		return fmt.Errorf("no check queries found in database")
	}
	
	log.Printf("Environment validation successful: %d check queries found", len(queries))
	return nil
}

type ProcessingStats struct {
	TotalRepositories     int
	CompletedRepositories int
}