package batch

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/naoya0117/shuron2025/api/internal/database"
)

type Analyzer struct {
	db                  *database.DB
	repositoryProcessor *RepositoryProcessor
	sessionID           string
	workDir             string
}

func NewAnalyzer(db *database.DB, repositoryProcessor *RepositoryProcessor, workDir string) *Analyzer {
	return &Analyzer{
		db:                  db,
		repositoryProcessor: repositoryProcessor,
		sessionID:           uuid.New().String(),
		workDir:             workDir,
	}
}

func (a *Analyzer) Run() error {
	ctx := context.Background()
	log.Printf("Starting batch analysis with session ID: %s", a.sessionID)

	if err := a.setupWorkDirectory(); err != nil {
		return fmt.Errorf("failed to setup work directory: %w", err)
	}

	if err := a.repositoryProcessor.ValidateEnvironment(); err != nil {
		return fmt.Errorf("environment validation failed: %w", err)
	}

	// Initialize rate limit handling
	if err := a.repositoryProcessor.geminiAnalyzer.StartAnalysisWithRateLimitHandling(ctx); err != nil {
		return fmt.Errorf("failed to initialize rate limit handling: %w", err)
	}
	
	repositories, err := a.db.GetUncheckedRepositories()
	if err != nil {
		return fmt.Errorf("failed to get unchecked repositories: %w", err)
	}
	
	if len(repositories) == 0 {
		log.Println("No unchecked repositories found")
		return nil
	}
	
	log.Printf("Found %d unchecked repositories to process", len(repositories))
	
	progress := database.BatchProgress{
		SessionID:             a.sessionID,
		TotalRepositories:     len(repositories),
		CompletedRepositories: 0,
		Status:                "running",
	}
	
	if err := a.db.SaveBatchProgress(progress); err != nil {
		return fmt.Errorf("failed to save initial progress: %w", err)
	}
	
	a.setupSignalHandler()
	
	startTime := time.Now()
	for i, repo := range repositories {
		log.Printf("Processing repository %d/%d: %s", i+1, len(repositories), repo.NameWithOwner)

		progress.CurrentRepositoryID = &repo.ID
		if err := a.db.SaveBatchProgress(progress); err != nil {
			log.Printf("Warning: failed to update progress: %v", err)
		}

		repoStartTime := time.Now()
		err := a.repositoryProcessor.ProcessRepository(ctx, repo)

		if err != nil {
			log.Printf("Failed to process repository %s: %v", repo.NameWithOwner, err)
			// Rate limit is handled internally now, so we don't need special handling here
			// The analyzer will sleep and retry automatically
		} else {
			progress.CompletedRepositories++
			log.Printf("Successfully processed repository %s (took %v)",
				repo.NameWithOwner, time.Since(repoStartTime))
		}

		if err := a.db.SaveBatchProgress(progress); err != nil {
			log.Printf("Warning: failed to update progress: %v", err)
		}

		elapsed := time.Since(startTime)
		remaining := len(repositories) - (i + 1)
		if remaining > 0 && i > 0 {
			avgTimePerRepo := elapsed / time.Duration(i+1)
			estimatedRemaining := avgTimePerRepo * time.Duration(remaining)
			log.Printf("Progress: %d/%d completed, estimated time remaining: %v",
				i+1, len(repositories), estimatedRemaining)
		}
	}
	
	progress.Status = "completed"
	progress.CurrentRepositoryID = nil
	if err := a.db.SaveBatchProgress(progress); err != nil {
		log.Printf("Warning: failed to update final progress: %v", err)
	}
	
	totalDuration := time.Since(startTime)
	log.Printf("Batch analysis completed in %v", totalDuration)
	log.Printf("Final results: %d/%d repositories processed successfully", 
		progress.CompletedRepositories, progress.TotalRepositories)
	
	return nil
}

func (a *Analyzer) Resume(sessionID string) error {
	ctx := context.Background()
	log.Printf("Resuming batch analysis with session ID: %s", sessionID)

	progress, err := a.db.LoadBatchProgress(sessionID)
	if err != nil {
		return fmt.Errorf("failed to load progress for session %s: %w", sessionID, err)
	}

	if progress == nil {
		return fmt.Errorf("no progress found for session %s", sessionID)
	}

	if progress.Status == "completed" {
		log.Printf("Session %s is already completed", sessionID)
		return nil
	}

	a.sessionID = sessionID

	// Initialize rate limit handling
	if err := a.repositoryProcessor.geminiAnalyzer.StartAnalysisWithRateLimitHandling(ctx); err != nil {
		return fmt.Errorf("failed to initialize rate limit handling: %w", err)
	}

	repositories, err := a.db.GetUncheckedRepositories()
	if err != nil {
		return fmt.Errorf("failed to get unchecked repositories: %w", err)
	}

	if len(repositories) == 0 {
		log.Println("No unchecked repositories found")
		progress.Status = "completed"
		if err := a.db.SaveBatchProgress(*progress); err != nil {
			log.Printf("Warning: failed to update progress: %v", err)
		}
		return nil
	}

	log.Printf("Resuming processing of %d remaining repositories", len(repositories))

	progress.Status = "running"
	progress.TotalRepositories = len(repositories) + progress.CompletedRepositories

	if err := a.db.SaveBatchProgress(*progress); err != nil {
		return fmt.Errorf("failed to save resumed progress: %w", err)
	}

	a.setupSignalHandler()

	for i, repo := range repositories {
		log.Printf("Processing repository %d/%d: %s", i+1, len(repositories), repo.NameWithOwner)

		progress.CurrentRepositoryID = &repo.ID
		if err := a.db.SaveBatchProgress(*progress); err != nil {
			log.Printf("Warning: failed to update progress: %v", err)
		}

		if err := a.repositoryProcessor.ProcessRepository(ctx, repo); err != nil {
			log.Printf("Failed to process repository %s: %v", repo.NameWithOwner, err)
		} else {
			progress.CompletedRepositories++
			log.Printf("Successfully processed repository %s", repo.NameWithOwner)
		}

		if err := a.db.SaveBatchProgress(*progress); err != nil {
			log.Printf("Warning: failed to update progress: %v", err)
		}
	}
	
	progress.Status = "completed"
	progress.CurrentRepositoryID = nil
	if err := a.db.SaveBatchProgress(*progress); err != nil {
		log.Printf("Warning: failed to update final progress: %v", err)
	}
	
	log.Printf("Resumed batch analysis completed")
	log.Printf("Final results: %d/%d repositories processed successfully", 
		progress.CompletedRepositories, progress.TotalRepositories)
	
	return nil
}

func (a *Analyzer) GetStatus() (*AnalysisStatus, error) {
	progressList, err := a.db.ListBatchProgress()
	if err != nil {
		return nil, fmt.Errorf("failed to get batch progress: %w", err)
	}
	
	repositories, err := a.db.GetUncheckedRepositories()
	if err != nil {
		return nil, fmt.Errorf("failed to get unchecked repositories: %w", err)
	}
	
	stats, err := a.repositoryProcessor.GetProcessingStats(repositories)
	if err != nil {
		return nil, fmt.Errorf("failed to get processing stats: %w", err)
	}
	
	return &AnalysisStatus{
		ActiveSessions:        progressList,
		UncheckedRepositories: len(repositories),
		ProcessingStats:       stats,
	}, nil
}

func (a *Analyzer) setupWorkDirectory() error {
	if err := os.MkdirAll(a.workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory %s: %w", a.workDir, err)
	}
	
	log.Printf("Work directory ready: %s", a.workDir)
	return nil
}

func (a *Analyzer) setupSignalHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		sig := <-c
		log.Printf("Received signal %v, saving progress and shutting down...", sig)
		
		progress, err := a.db.LoadBatchProgress(a.sessionID)
		if err != nil {
			log.Printf("Failed to load progress during shutdown: %v", err)
			os.Exit(1)
		}
		
		if progress != nil && progress.Status == "running" {
			progress.Status = "paused"
			if err := a.db.SaveBatchProgress(*progress); err != nil {
				log.Printf("Failed to save progress during shutdown: %v", err)
			} else {
				log.Printf("Progress saved. Use --resume --session-id=%s to continue", a.sessionID)
			}
		}
		
		os.Exit(0)
	}()
}

type AnalysisStatus struct {
	ActiveSessions        []database.BatchProgress
	UncheckedRepositories int
	ProcessingStats       *ProcessingStats
}