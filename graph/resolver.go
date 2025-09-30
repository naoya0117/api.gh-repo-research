package graph

import "github.com/naoya0117/shuron2025/api/internal/database"

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	DB *database.DB
}
