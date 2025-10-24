package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/naoya0117/shuron2025/api/internal/database"
)

// connectDatabase opens a database connection using either the provided URL or DATABASE_URL.
func connectDatabase(dbURL string) (*database.DB, error) {
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
