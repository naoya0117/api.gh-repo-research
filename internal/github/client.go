package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
}

type RateLimitError struct {
	Message   string
	ResetTime time.Time
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded: %s (resets at %s)", e.Message, e.ResetTime.Format("2006-01-02 15:04:05 MST"))
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

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == 429 {
			return c.handleRateLimit(resp, body)
		}
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var gqlResp GraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		for _, err := range gqlResp.Errors {
			if strings.Contains(strings.ToLower(err.Message), "rate limit") || err.Type == "RATE_LIMITED" {
				return c.parseRateLimitFromError(err.Message)
			}
		}
		return fmt.Errorf("GraphQL errors: %+v", gqlResp.Errors)
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

func (c *Client) handleRateLimit(resp *http.Response, _ []byte) error {
	resetHeader := resp.Header.Get("X-RateLimit-Reset")
	if resetHeader != "" {
		return &RateLimitError{
			Message:   "Rate limit exceeded",
			ResetTime: calculateNextMidnight(),
		}
	}

	return &RateLimitError{
		Message:   "Rate limit exceeded",
		ResetTime: calculateNextMidnight(),
	}
}

func (c *Client) parseRateLimitFromError(message string) error {
	return &RateLimitError{
		Message:   message,
		ResetTime: calculateNextMidnight(),
	}
}

func calculateNextMidnight() time.Time {
	now := time.Now()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return nextMidnight
}

func IsRateLimitError(err error) bool {
	_, ok := err.(*RateLimitError)
	return ok
}
