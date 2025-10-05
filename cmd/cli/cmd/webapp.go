package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var webappDbURL string

var webappCmd = &cobra.Command{
	Use:   "webapp",
	Short: "Web app check operations",
	Long:  `Manage repository web app checks table including creation and initialization.`,
}

var webappInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize repository web app checks table",
	Run: func(cmd *cobra.Command, args []string) {
		initWebAppCheckTable()
	},
}

func init() {
	webappCmd.AddCommand(webappInitCmd)
	webappCmd.PersistentFlags().StringVar(&webappDbURL, "db-url", "", "Database URL (if not set, uses DATABASE_URL environment variable)")
}

func initWebAppCheckTable() {
	db, err := setupBatchDatabase(webappDbURL)
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer db.Close()

	log.Println("Creating repository_webapp_checks table...")

	query := `
		CREATE TABLE IF NOT EXISTS repository_webapp_checks (
			id SERIAL PRIMARY KEY,
			repository_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
			is_web_app BOOLEAN,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(repository_id)
		)
	`

	if _, err := db.Exec(query); err != nil {
		log.Fatalf("Failed to create repository_webapp_checks table: %v", err)
	}

	log.Println("✓ repository_webapp_checks table created successfully")
	fmt.Println("Table structure:")
	fmt.Println("  - id: SERIAL PRIMARY KEY")
	fmt.Println("  - repository_id: INTEGER (FOREIGN KEY to repositories.id)")
	fmt.Println("  - is_web_app: BOOLEAN")
	fmt.Println("  - created_at: TIMESTAMP")
	fmt.Println("  - updated_at: TIMESTAMP")
	fmt.Println("  - UNIQUE constraint on repository_id")
}
