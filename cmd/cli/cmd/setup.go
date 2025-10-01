package cmd

import (
	"log"
	"os"

	"github.com/naoya0117/shuron2025/api/internal/database"
	"github.com/spf13/cobra"
)

var setupDbURL string

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup operations",
	Long:  `Perform setup operations like initializing check queries.`,
}

var setupQueriesCmd = &cobra.Command{
	Use:   "queries",
	Short: "Setup sample check queries",
	Run: func(cmd *cobra.Command, args []string) {
		setupCheckQueries()
	},
}

func init() {
	setupCmd.AddCommand(setupQueriesCmd)
	setupCmd.PersistentFlags().StringVar(&setupDbURL, "db-url", "", "Database URL (if not set, uses DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME environment variables)")
}

func setupCheckQueries() {
	if setupDbURL == "" {
		setupDbURL = os.Getenv("DATABASE_URL")
	}

	db, err := database.Connect(setupDbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create necessary tables
	log.Println("Creating necessary tables...")
	if err := db.CreateRepositoriesTable(); err != nil {
		log.Printf("Warning: Failed to create repositories table: %v", err)
	}
	if err := db.CreateCheckQueriesTable(); err != nil {
		log.Fatalf("Failed to create check_queries table: %v", err)
	}
	if err := db.CreateEasyCheckedRepositoriesTable(); err != nil {
		log.Printf("Warning: Failed to create easy_checked_repositories table: %v", err)
	}
	if err := db.CreateBatchProgressTable(); err != nil {
		log.Printf("Warning: Failed to create batch_progress table: %v", err)
	}

	log.Println("Setting up sample check queries...")

	queries := []struct {
		name        string
		description string
		template    string
	}{
		{
			name:        "Dockerfile Analysis",
			description: "Dockerfileの内容を分析し、ベストプラクティスの遵守状況を評価します。",
			template:    "このリポジトリのDockerfileを分析してください。セキュリティ上の問題、ベストプラクティスの遵守状況、改善提案があれば教えてください。",
		},
		{
			name:        "README Quality Assessment",
			description: "READMEファイルの品質と完成度を評価します。",
			template:    "このリポジトリのREADMEファイルを評価してください。プロジェクトの説明、インストール方法、使用方法の記載状況を分析し、改善点があれば提案してください。",
		},
		{
			name:        "Technology Stack Analysis",
			description: "使用されている技術スタックと依存関係を分析します。",
			template:    "このリポジトリで使用されている技術スタック（プログラミング言語、フレームワーク、ライブラリ）を特定し、現代的な技術の使用状況を評価してください。",
		},
		{
			name:        "Project Structure Assessment",
			description: "プロジェクトの構造とディレクトリ組織を評価します。",
			template:    "このリポジトリのプロジェクト構造を分析してください。ディレクトリの組織化、ファイルの配置、全体的な構造の適切性を評価し、改善提案があれば教えてください。",
		},
		{
			name:        "Security Analysis",
			description: "セキュリティ上の懸念事項や脆弱性を分析します。",
			template:    "このリポジトリをセキュリティの観点から分析してください。潜在的な脆弱性、機密情報の漏洩リスク、セキュリティベストプラクティスの遵守状況を評価してください。",
		},
	}

	for _, query := range queries {
		if err := db.InsertCheckQuery(query.name, query.description, query.template); err != nil {
			log.Printf("Failed to insert query '%s': %v", query.name, err)
		} else {
			log.Printf("✓ Inserted check query: %s", query.name)
		}
	}

	log.Println("Setup completed successfully!")
}