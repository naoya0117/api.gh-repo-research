package middleware

import (
	"net/http"
	"os"
	"strings"
)

// APIKeyAuth is a middleware that validates API key authentication
// for internal communication between Next.js and the Go backend.
//
// It checks the Authorization header for a Bearer token and compares it
// with the API_SECRET_KEY environment variable.
//
// If the key is not set or doesn't match, it returns 401 Unauthorized.
func APIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := os.Getenv("API_SECRET_KEY")

		// If API_SECRET_KEY is not set, deny all requests for security
		if apiKey == "" {
			http.Error(w, "API authentication is not configured", http.StatusInternalServerError)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Expected format: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		token := parts[1]
		if token != apiKey {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		// Authentication successful, proceed to next handler
		next.ServeHTTP(w, r)
	})
}
