package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/naoya0117/shuron2025/api/graph"
	"github.com/naoya0117/shuron2025/api/internal/database"
	"github.com/naoya0117/shuron2025/api/internal/middleware"
	adminserver "github.com/naoya0117/shuron2025/api/internal/server"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/vektah/gqlparser/v2/ast"
)

const defaultPort = "8080"

// main is the entry point of the program.
//
// It sets up a GraphQL endpoint and an admin interface for managing check queries.
//
// The GraphQL endpoint is available at http://localhost:8080/query and the admin interface is available at http://localhost:8080/admin/check-queries and http://localhost:8080/admin/check-results.
//
// If the ENV environment variable is not set to "production", a GraphQL playground is available at http://localhost:8080/.
func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	// Initialize database connection
	db, err := database.NewConnection()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database connection: %v", err)
		}
	}()

	if err := db.EnsureCoreTables(); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	srv := handler.New(graph.NewExecutableSchema(graph.Config{Resolvers: &graph.Resolver{DB: db}}))

	adminSrv, err := adminserver.NewAdminServer(db)
	if err != nil {
		log.Fatalf("Failed to initialize admin server: %v", err)
	}

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	http.HandleFunc("/admin/check-queries", adminSrv.HandleCheckQueries)
	http.HandleFunc("/admin/check-results", adminSrv.HandleCheckResults)

	if env := os.Getenv("ENV"); strings.ToLower(env) != "production" {
		http.Handle("/", playground.Handler("GraphQL playground", "/query"))
		log.Printf("GraphQL playground available at http://localhost:%s/", port)
	}

	// Apply API key authentication middleware to GraphQL endpoint
	http.Handle("/query", middleware.APIKeyAuth(srv))

	log.Printf("GraphQL endpoint available at http://localhost:%s/query", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
