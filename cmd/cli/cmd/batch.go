package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/batch"
	"github.com/naoya0117/shuron2025/api/internal/database"
	"github.com/spf13/cobra"
)

var (
	batchWorkDir   string
	batchDbURL     string
	batchSessionID string
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Batch analysis operations",
	Long:  `Perform batch analysis operations on repositories including start, resume, and status commands.`,
}

var batchStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start new batch analysis",
	Run: func(cmd *cobra.Command, args []string) {
		runBatchAnalysis(false, "")
	},
}

var batchResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume batch analysis",
	Run: func(cmd *cobra.Command, args []string) {
		if batchSessionID == "" {
			log.Fatal("--session-id is required when using resume")
		}
		runBatchAnalysis(true, batchSessionID)
	},
}

var batchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current status",
	Run: func(cmd *cobra.Command, args []string) {
		showBatchStatus()
	},
}

func init() {
	batchCmd.AddCommand(batchStartCmd)
	batchCmd.AddCommand(batchResumeCmd)
	batchCmd.AddCommand(batchStatusCmd)

	batchCmd.PersistentFlags().StringVar(&batchWorkDir, "work-dir", "tmp_repositories", "Working directory for repositories")
	batchCmd.PersistentFlags().StringVar(&batchDbURL, "db-url", "", "Database URL (if not set, uses DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME environment variables)")
	batchResumeCmd.Flags().StringVar(&batchSessionID, "session-id", "", "Session ID for resume operation")
}

func runBatchAnalysis(resume bool, sessionID string) {
	workDirAbs, err := filepath.Abs(batchWorkDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path for work directory: %v", err)
	}

	log.Printf("Batch Analyzer starting...")
	log.Printf("Work directory: %s", workDirAbs)

	db, err := setupBatchDatabase(batchDbURL)
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer db.Close()

	if err := createBatchTables(db); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	geminiClient := batch.NewGeminiClient(workDirAbs, 60*time.Second)
	gitManager := batch.NewGitManager(workDirAbs, 30*time.Second)
	repositoryProcessor := batch.NewRepositoryProcessor(db, geminiClient, gitManager, workDirAbs)
	analyzer := batch.NewAnalyzer(db, repositoryProcessor, workDirAbs)

	if resume {
		log.Printf("Resuming batch analysis with session ID: %s", sessionID)
		if err := analyzer.Resume(sessionID); err != nil {
			log.Fatalf("Failed to resume batch analysis: %v", err)
		}
	} else {
		log.Println("Starting new batch analysis...")
		if err := analyzer.Run(); err != nil {
			log.Fatalf("Batch analysis failed: %v", err)
		}
	}

	log.Println("Batch analyzer completed successfully")
}

func showBatchStatus() {
	db, err := setupBatchDatabase(batchDbURL)
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer db.Close()

	workDirAbs, err := filepath.Abs(batchWorkDir)
	if err != nil {
		log.Fatalf("Failed to get absolute path for work directory: %v", err)
	}

	geminiClient := batch.NewGeminiClient(workDirAbs, 60*time.Second)
	gitManager := batch.NewGitManager(workDirAbs, 30*time.Second)
	repositoryProcessor := batch.NewRepositoryProcessor(db, geminiClient, gitManager, workDirAbs)
	analyzer := batch.NewAnalyzer(db, repositoryProcessor, workDirAbs)

	status, err := analyzer.GetStatus()
	if err != nil {
		log.Fatalf("Failed to get status: %v", err)
	}
	printBatchStatus(status)
}

func setupBatchDatabase(dbURL string) (*database.DB, error) {
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	log.Println("✓ Database connection established")
	return db, nil
}

func createBatchTables(db *database.DB) error {
	log.Println("Creating database tables...")

	if err := db.CreateRepositoriesTable(); err != nil {
		return fmt.Errorf("failed to create repositories table: %w", err)
	}

	if err := db.CreateSearchStatesTable(); err != nil {
		return fmt.Errorf("failed to create search_states table: %w", err)
	}

	if err := db.CreateCheckQueriesTable(); err != nil {
		return fmt.Errorf("failed to create check_queries table: %w", err)
	}

	if err := db.CreateEasyCheckedRepositoriesTable(); err != nil {
		return fmt.Errorf("failed to create easy_checked_repositories table: %w", err)
	}

	if err := db.CreateBatchProgressTable(); err != nil {
		return fmt.Errorf("failed to create batch_progress table: %w", err)
	}

	log.Println("✓ All database tables created/verified")
	return nil
}

func printBatchStatus(status *batch.AnalysisStatus) {
	fmt.Println("=== Batch Analysis Status ===")
	fmt.Printf("Unchecked repositories: %d\n", status.UncheckedRepositories)
	fmt.Printf("Total repositories: %d\n", status.ProcessingStats.TotalRepositories)
	fmt.Printf("Completed repositories: %d\n", status.ProcessingStats.CompletedRepositories)

	if len(status.ActiveSessions) == 0 {
		fmt.Println("No active sessions")
		return
	}

	fmt.Println("\nActive Sessions:")
	for _, session := range status.ActiveSessions {
		fmt.Printf("- Session: %s\n", session.SessionID)
		fmt.Printf("  Status: %s\n", session.Status)
		fmt.Printf("  Progress: %d/%d repositories\n", session.CompletedRepositories, session.TotalRepositories)
		fmt.Printf("  Started: %s\n", session.StartedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Updated: %s\n", session.UpdatedAt.Format("2006-01-02 15:04:05"))

		if session.CurrentRepositoryID != nil {
			fmt.Printf("  Current Repository ID: %d\n", *session.CurrentRepositoryID)
		}

		if session.RateLimitResetTime != nil {
			fmt.Printf("  Rate Limit Reset Time: %s\n", session.RateLimitResetTime.Format("2006-01-02 15:04:05"))
			remaining := time.Until(*session.RateLimitResetTime)
			if remaining > 0 {
				fmt.Printf("  Time until reset: %v\n", remaining.Round(time.Minute))
			}
		}

		completion := float64(session.CompletedRepositories) / float64(session.TotalRepositories) * 100
		fmt.Printf("  Completion: %.1f%%\n", completion)
		fmt.Println()
	}
}
