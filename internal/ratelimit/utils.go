package ratelimit

import (
	"context"
	"errors"
	"log"
	"time"
)

var (
	ErrRateLimited = errors.New("rate limit exceeded")
)

// NextJSTMidnight calculates the next midnight in JST timezone
func NextJSTMidnight(now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		log.Printf("Warning: failed to load Asia/Tokyo timezone, using UTC: %v", err)
		loc = time.UTC
	}

	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, loc)
}

// ChooseResumeAt selects the appropriate resume time
// Priority: provider's resetAt if valid and in the future, otherwise JST midnight
func ChooseResumeAt(now time.Time, providerResetAt *time.Time) time.Time {
	if providerResetAt != nil && providerResetAt.After(now) {
		log.Printf("Using provider reset time: %s", providerResetAt.Format("2006-01-02 15:04:05 MST"))
		return *providerResetAt
	}

	jstMidnight := NextJSTMidnight(now)
	log.Printf("Using JST midnight: %s", jstMidnight.Format("2006-01-02 15:04:05 MST"))
	return jstMidnight
}

// SleepUntilCtx sleeps until the specified time or context is cancelled
func SleepUntilCtx(ctx context.Context, targetTime time.Time) error {
	duration := time.Until(targetTime)
	if duration <= 0 {
		log.Printf("Target time %s has already passed, no sleep needed", targetTime.Format("2006-01-02 15:04:05 MST"))
		return nil
	}

	log.Printf("Sleeping for %v until %s", duration.Round(time.Second), targetTime.Format("2006-01-02 15:04:05 MST"))

	// Progress logging ticker
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	timer := time.NewTimer(duration)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Sleep cancelled due to context cancellation")
			return ctx.Err()
		case <-timer.C:
			log.Printf("Sleep completed, resuming operations at %s", time.Now().Format("2006-01-02 15:04:05 MST"))
			return nil
		case <-ticker.C:
			remaining := time.Until(targetTime)
			if remaining <= 0 {
				return nil
			}
			log.Printf("Rate limit wait in progress. Time remaining: %v (Target: %s)",
				remaining.Round(time.Minute), targetTime.Format("2006-01-02 15:04:05 MST"))
		}
	}
}

// ParseGitHubResetAt extracts the reset time from GitHub API response
func ParseGitHubResetAt(resetAtStr string) (*time.Time, error) {
	if resetAtStr == "" {
		return nil, nil
	}

	// GitHub returns ISO 8601 format: 2024-01-01T00:00:00Z
	t, err := time.Parse(time.RFC3339, resetAtStr)
	if err != nil {
		return nil, err
	}

	return &t, nil
}
