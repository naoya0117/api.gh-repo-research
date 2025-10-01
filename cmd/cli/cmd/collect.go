package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/database"
	"github.com/naoya0117/shuron2025/api/internal/github"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	collectSessionID  string
	collectQuery      string
	collectMaxRepos   int
	collectDeleteID   string
)

var collectCmd = &cobra.Command{
	Use:   "collect",
	Short: "GitHub repository collection operations",
	Long:  `Collect GitHub repositories, manage collection sessions, and view collection status.`,
}

var collectStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start collecting GitHub repositories",
	Run: func(cmd *cobra.Command, args []string) {
		runGitHubCollection()
	},
}

var collectResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a collection session",
	Run: func(cmd *cobra.Command, args []string) {
		if collectSessionID == "" {
			log.Fatal("--session-id is required when using resume")
		}
		runGitHubCollection()
	},
}

var collectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all saved collection sessions",
	Run: func(cmd *cobra.Command, args []string) {
		listCollectionSessions()
	},
}

var collectDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a collection session",
	Run: func(cmd *cobra.Command, args []string) {
		if collectDeleteID == "" {
			log.Fatal("--session-id is required when using delete")
		}
		deleteCollectionSession(collectDeleteID)
	},
}

func init() {
	collectCmd.AddCommand(collectStartCmd)
	collectCmd.AddCommand(collectResumeCmd)
	collectCmd.AddCommand(collectListCmd)
	collectCmd.AddCommand(collectDeleteCmd)

	collectStartCmd.Flags().StringVar(&collectQuery, "query", "docker sort:stars-desc in:readme", "GitHub search query")
	collectStartCmd.Flags().IntVar(&collectMaxRepos, "max", 0, "Maximum number of repositories to fetch (0 = unlimited)")

	collectResumeCmd.Flags().StringVar(&collectSessionID, "session-id", "", "Session ID to resume")

	collectDeleteCmd.Flags().StringVar(&collectDeleteID, "session-id", "", "Session ID to delete")
}

func runGitHubCollection() {
	client := github.NewClient(os.Getenv("GITHUB_TOKEN"))

	// Initialize database connection
	db, err := database.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create tables
	if err := db.CreateRepositoriesTable(); err != nil {
		log.Fatalf("Failed to create repositories table: %v", err)
	}
	if err := db.CreateSearchStatesTable(); err != nil {
		log.Fatalf("Failed to create search_states table: %v", err)
	}

	ctx := context.Background()
	var currentSessionID string
	var currentCursor string
	var totalFetched int

	// Load or create search state
	if collectSessionID != "" {
		// Resume existing session
		state, err := db.LoadSearchState(collectSessionID)
		if err != nil {
			log.Fatalf("Failed to load search state: %v", err)
		}
		if state == nil {
			log.Fatalf("Search state with session ID '%s' not found", collectSessionID)
		}

		if state.IsCompleted {
			fmt.Printf("Search session '%s' is already completed (%d repositories fetched).\n",
				collectSessionID, state.TotalFetched)
			return
		}

		currentSessionID = state.SessionID
		if state.CurrentCursor != nil {
			currentCursor = *state.CurrentCursor
		}
		totalFetched = state.TotalFetched
		collectQuery = state.Query

		fmt.Printf("Resuming search session: %s\n", currentSessionID)
		fmt.Printf("Query: %s\n", collectQuery)
		fmt.Printf("Already fetched: %d repositories\n", totalFetched)
		if currentCursor != "" {
			fmt.Printf("Resuming from cursor: %.50s...\n", currentCursor)
		}
		fmt.Println()

	} else {
		// Create new session
		currentSessionID = uuid.New().String()
		totalFetched = 0
		currentCursor = ""

		// Save initial state
		initialState := database.SearchState{
			SessionID:     currentSessionID,
			Query:         collectQuery,
			CurrentCursor: nil,
			TotalFetched:  0,
			IsCompleted:   false,
		}

		if err := db.SaveSearchState(initialState); err != nil {
			log.Fatalf("Failed to save initial search state: %v", err)
		}

		fmt.Printf("Starting new search session: %s\n", currentSessionID)
		fmt.Printf("Query: %s\n", collectQuery)
		fmt.Println()
	}

	pageCount := 0
	for {
		result, err := client.GetNextRepositories(ctx, currentCursor)
		if err != nil {
			if github.IsRateLimitError(err) {
				fmt.Printf("🚫 %v\n", err)
				
				// Save current state before waiting
				state := database.SearchState{
					SessionID:     currentSessionID,
					Query:         collectQuery,
					CurrentCursor: &currentCursor,
					TotalFetched:  totalFetched,
					IsCompleted:   false,
				}
				if saveErr := db.SaveSearchState(state); saveErr != nil {
					log.Printf("Failed to save search state: %v", saveErr)
				}
				
				fmt.Printf("💾 Current progress saved (session: %s)\n", currentSessionID)
				
				// Wait until midnight and continue
				github.WaitUntilMidnight()
				
				fmt.Printf("🔄 Resuming collection...\n")
				continue
			}
			
			log.Printf("Failed to search repositories: %v", err)

			// Save current state before exit
			state := database.SearchState{
				SessionID:     currentSessionID,
				Query:         collectQuery,
				CurrentCursor: &currentCursor,
				TotalFetched:  totalFetched,
				IsCompleted:   false,
			}
			if saveErr := db.SaveSearchState(state); saveErr != nil {
				log.Printf("Failed to save search state: %v", saveErr)
			}

			log.Fatalf("Search failed. State saved. You can resume with: resume --session-id=%s", currentSessionID)
		}

		pageCount++
		fmt.Printf("Page %d: Found %d repositories\n", pageCount, len(result.Repositories))

		for _, repo := range result.Repositories {
			parts := strings.Split(repo.FullName, "/")
			if len(parts) != 2 {
				continue
			}
			owner, name := parts[0], parts[1]

			hasDockerfile, err := client.HasDockerfile(ctx, owner, name)
			if err != nil {
				if github.IsRateLimitError(err) {
					fmt.Printf("🚫 Rate limit while checking Dockerfile for %s. Skipping...\n", repo.FullName)
					hasDockerfile = false
				} else {
					fmt.Printf("Error checking Dockerfile for %s: %v\n", repo.FullName, err)
					hasDockerfile = false
				}
			}

			// Convert GitHub repo to database repo
			dbRepo := database.Repository{
				URL:            repo.URL,
				NameWithOwner:  repo.FullName,
				StargazerCount: repo.StargazerCount,
				HasDockerfile:  hasDockerfile,
			}

			// Set primary language if available
			if repo.PrimaryLanguage != nil {
				dbRepo.PrimaryLanguage = &repo.PrimaryLanguage.Name
			}

			// Save to database
			if err := db.InsertRepository(dbRepo); err != nil {
				fmt.Printf("Error saving repository %s: %v\n", repo.FullName, err)
				continue
			}

			totalFetched++

			status := ""
			if hasDockerfile {
				status = " [HAS DOCKERFILE]"
			}

			language := "Unknown"
			if repo.PrimaryLanguage != nil {
				language = repo.PrimaryLanguage.Name
			}

			fmt.Printf("  - %s (%d stars, %s)%s - SAVED\n", repo.FullName, repo.StargazerCount, language, status)
		}

		// Update cursor
		if result.PageInfo.EndCursor != nil {
			currentCursor = *result.PageInfo.EndCursor
		}

		// Save current state after each page
		state := database.SearchState{
			SessionID:     currentSessionID,
			Query:         collectQuery,
			CurrentCursor: &currentCursor,
			TotalFetched:  totalFetched,
			IsCompleted:   !result.PageInfo.HasNextPage,
		}

		if err := db.SaveSearchState(state); err != nil {
			log.Printf("Failed to save search state: %v", err)
		}

		fmt.Printf("Progress: %d repositories fetched (session: %s)\n\n", totalFetched, currentSessionID)

		// Check if we should stop
		if !result.PageInfo.HasNextPage {
			fmt.Println("🎉 Collection completed! All repositories have been fetched.")
			break
		}

		if result.PageInfo.EndCursor == nil {
			fmt.Println("⚠️  No more pages available (cursor is nil).")
			break
		}

		// Check max limit
		if collectMaxRepos > 0 && totalFetched >= collectMaxRepos {
			fmt.Printf("🛑 Reached maximum limit of %d repositories.\n", collectMaxRepos)

			// Mark as completed since we reached the user-defined limit
			state.IsCompleted = true
			if err := db.SaveSearchState(state); err != nil {
				log.Printf("Failed to save final search state: %v", err)
			}
			break
		}

		// Add a small delay to be respectful to the API
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\n✅ Session %s finished. Total repositories collected: %d\n", currentSessionID, totalFetched)

	// Show how to resume if interrupted
	if !(collectMaxRepos > 0 && totalFetched >= collectMaxRepos) {
		fmt.Printf("💡 To resume this search later, use: resume --session-id=%s\n", currentSessionID)
		fmt.Printf("💡 To list all sessions, use: list\n")
		fmt.Printf("💡 To delete this session, use: delete --session-id=%s\n", currentSessionID)
	}
}

func listCollectionSessions() {
	db, err := database.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	states, err := db.ListSearchStates()
	if err != nil {
		log.Fatalf("Failed to list search states: %v", err)
	}

	if len(states) == 0 {
		fmt.Println("No saved search states found.")
		return
	}

	fmt.Printf("Found %d saved search state(s):\n\n", len(states))
	for _, state := range states {
		status := "In Progress"
		if state.IsCompleted {
			status = "Completed"
		}

		cursor := "None"
		if state.CurrentCursor != nil {
			cursor = fmt.Sprintf("%.20s...", *state.CurrentCursor)
		}

		fmt.Printf("Session ID: %s\n", state.SessionID)
		fmt.Printf("  Query: %s\n", state.Query)
		fmt.Printf("  Status: %s\n", status)
		fmt.Printf("  Total Fetched: %d repositories\n", state.TotalFetched)
		fmt.Printf("  Current Cursor: %s\n", cursor)
		fmt.Printf("  Last Updated: %s\n\n", state.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
}

func deleteCollectionSession(sessionID string) {
	db, err := database.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.DeleteSearchState(sessionID); err != nil {
		log.Fatalf("Failed to delete search state: %v", err)
	}
	fmt.Printf("Search state with session ID '%s' deleted successfully.\n", sessionID)
}