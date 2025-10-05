package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var mychkDbURL string

var mychkCmd = &cobra.Command{
	Use:   "mychk",
	Short: "My checked repositories operations",
	Long:  `Manage my_checked_repositories table including creation and initialization.`,
}

var mychkInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize my_checked_repositories table",
	Run: func(cmd *cobra.Command, args []string) {
		initMyCheckedRepositoriesTable()
	},
}

func init() {
	mychkCmd.AddCommand(mychkInitCmd)
	mychkCmd.PersistentFlags().StringVar(&mychkDbURL, "db-url", "", "Database URL (if not set, uses DATABASE_URL environment variable)")
}

func initMyCheckedRepositoriesTable() {
	db, err := setupBatchDatabase(mychkDbURL)
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer db.Close()

	log.Println("Creating my_checked_repositories table...")

	query := `
		CREATE TABLE IF NOT EXISTS my_checked_repositories (
			id SERIAL PRIMARY KEY,
			repository_id INTEGER REFERENCES repositories(id),
			check_query_id INTEGER REFERENCES check_queries(id),
			memo TEXT,
			result TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(repository_id, check_query_id)
		)
	`

	if _, err := db.Exec(query); err != nil {
		log.Fatalf("Failed to create my_checked_repositories table: %v", err)
	}

	log.Println("✓ my_checked_repositories table created successfully")
	fmt.Println("Table structure:")
	fmt.Println("  - id: SERIAL PRIMARY KEY")
	fmt.Println("  - repository_id: INTEGER (FOREIGN KEY to repositories.id)")
	fmt.Println("  - check_query_id: INTEGER (FOREIGN KEY to check_queries.id)")
	fmt.Println("  - memo: TEXT")
	fmt.Println("  - result: TEXT")
	fmt.Println("  - created_at: TIMESTAMP")
	fmt.Println("  - updated_at: TIMESTAMP")
	fmt.Println("  - UNIQUE constraint on (repository_id, check_query_id)")
}
