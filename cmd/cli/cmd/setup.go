package cmd

import (
	"log"
	"strings"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/database"
	"github.com/naoya0117/shuron2025/api/internal/discord"
	"github.com/spf13/cobra"
)

var setupDbURL string

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup operations",
	Long:  `Perform setup operations like initializing check queries.`,
}

var setupTablesCmd = &cobra.Command{
	Use:   "tables",
	Short: "Create all required database tables",
	Run: func(cmd *cobra.Command, args []string) {
		setupTables()
	},
}

var setupQueriesCmd = &cobra.Command{
	Use:   "queries",
	Short: "Setup sample check queries",
	Run: func(cmd *cobra.Command, args []string) {
		setupCheckQueries()
	},
}

func init() {
	setupCmd.AddCommand(setupTablesCmd)
	setupCmd.AddCommand(setupQueriesCmd)
	setupCmd.PersistentFlags().StringVar(&setupDbURL, "db-url", "", "Database URL (if not set, uses DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME environment variables)")
}

func setupTables() {
	startTime := time.Now()

	db, err := connectDatabase(setupDbURL)
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer db.Close()

	if err := db.EnsureCoreTables(); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	log.Printf("✓ Tables ensured: %s", strings.Join(database.CoreTableNames, ", "))
	log.Printf("Completed in %v", time.Since(startTime))
}

func setupCheckQueries() {
	startTime := time.Now()

	db, err := connectDatabase(setupDbURL)
	if err != nil {
		log.Fatalf("Database setup failed: %v", err)
	}
	defer db.Close()

	if err := db.EnsureCoreTables(); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	log.Println("Setting up sample check queries...")

	queries := []struct {
		name        string
		description string
	}{
		{
			name:        "Dockerfile Analysis",
			description: "Dockerfileの内容を分析し、ベストプラクティスの遵守状況を評価します。",
		},
		{
			name:        "README Quality Assessment",
			description: "READMEファイルの品質と完成度を評価します。",
		},
		{
			name:        "Technology Stack Analysis",
			description: "使用されている技術スタックと依存関係を分析します。",
		},
		{
			name:        "Project Structure Assessment",
			description: "プロジェクトの構造とディレクトリ組織を評価します。",
		},
		{
			name:        "Security Analysis",
			description: "セキュリティ上の懸念事項や脆弱性を分析します。",
		},
	}

	for _, query := range queries {
		desc := strings.TrimSpace(query.description)
		var descPtr *string
		if desc != "" {
			descPtr = &desc
		}

		if _, err := db.InsertCheckQuery(query.name, descPtr); err != nil {
			log.Printf("Failed to insert query '%s': %v", query.name, err)
		} else {
			log.Printf("✓ Inserted check query: %s", query.name)
		}
	}

	duration := time.Since(startTime)
	log.Println("Setup completed successfully!")

	discordClient := discord.NewClient()
	if err := discordClient.SendCompletionNotification("setup queries", duration); err != nil {
		log.Printf("Failed to send Discord notification: %v", err)
	} else {
		log.Println("Discord notification sent successfully")
	}
}
