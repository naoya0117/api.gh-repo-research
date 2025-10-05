package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/ratelimit"
)

const GitHubGraphQLEndpoint = "https://api.github.com/graphql"

type Client struct {
	httpClient *http.Client
	token      string
}

func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
	Data   interface{} `json:"data"`
	Errors []struct {
		Message string        `json:"message"`
		Path    []interface{} `json:"path,omitempty"`
		Type    string        `json:"type,omitempty"`
	} `json:"errors,omitempty"`
	RateLimit *RateLimitInfo `json:"rateLimit,omitempty"`
}

type RateLimitInfo struct {
	Remaining int    `json:"remaining"`
	ResetAt   string `json:"resetAt"`
	Limit     int    `json:"limit"`
}

type RateLimitError struct {
	Message   string
	ResetTime *time.Time
}

func (e *RateLimitError) Error() string {
	if e.ResetTime != nil {
		return fmt.Sprintf("rate limit exceeded: %s (resets at %s)", e.Message, e.ResetTime.Format("2006-01-02 15:04:05 MST"))
	}
	return fmt.Sprintf("rate limit exceeded: %s", e.Message)
}

func (e *RateLimitError) Is(target error) bool {
	return target == ratelimit.ErrRateLimited
}

func (c *Client) Query(ctx context.Context, query string, variables map[string]interface{}, result interface{}) error {
	req := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", GitHubGraphQLEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "gh-repo-research-api/1.0")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for rate limit in response status code
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == 429 {
		return c.handleRateLimit(resp, &gqlResp)
	}

	// Check for rate limit in GraphQL errors
	if len(gqlResp.Errors) > 0 {
		for _, err := range gqlResp.Errors {
			if strings.Contains(strings.ToLower(err.Message), "rate limit") || err.Type == "RATE_LIMITED" {
				return c.parseRateLimitFromError(err.Message, &gqlResp)
			}
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code %d with errors: %+v", resp.StatusCode, gqlResp.Errors)
		}
		return fmt.Errorf("GraphQL errors: %+v", gqlResp.Errors)
	}

	// Check if rate limit is exhausted even on success
	if gqlResp.RateLimit != nil && gqlResp.RateLimit.Remaining == 0 {
		resetAt, _ := ratelimit.ParseGitHubResetAt(gqlResp.RateLimit.ResetAt)
		return &RateLimitError{
			Message:   "GraphQL rate limit remaining is 0",
			ResetTime: resetAt,
		}
	}

	dataBytes, err := json.Marshal(gqlResp.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, result); err != nil {
		return fmt.Errorf("failed to unmarshal data into result: %w", err)
	}

	return nil
}

func (c *Client) handleRateLimit(resp *http.Response, gqlResp *GraphQLResponse) error {
	var resetAt *time.Time

	// Try to extract reset time from GraphQL response
	if gqlResp != nil && gqlResp.RateLimit != nil {
		parsed, err := ratelimit.ParseGitHubResetAt(gqlResp.RateLimit.ResetAt)
		if err == nil {
			resetAt = parsed
		}
	}

	// Fallback: try HTTP headers
	if resetAt == nil {
		resetHeader := resp.Header.Get("X-RateLimit-Reset")
		if resetHeader != "" {
			// X-RateLimit-Reset is Unix timestamp
			var unixTime int64
			if _, err := fmt.Sscanf(resetHeader, "%d", &unixTime); err == nil {
				t := time.Unix(unixTime, 0)
				resetAt = &t
			}
		}
	}

	return &RateLimitError{
		Message:   "Rate limit exceeded via HTTP status",
		ResetTime: resetAt,
	}
}

func (c *Client) parseRateLimitFromError(message string, gqlResp *GraphQLResponse) error {
	var resetAt *time.Time

	if gqlResp != nil && gqlResp.RateLimit != nil {
		parsed, err := ratelimit.ParseGitHubResetAt(gqlResp.RateLimit.ResetAt)
		if err == nil {
			resetAt = parsed
		}
	}

	return &RateLimitError{
		Message:   message,
		ResetTime: resetAt,
	}
}

func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ratelimit.ErrRateLimited)
}
