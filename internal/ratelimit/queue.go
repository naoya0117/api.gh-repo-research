package ratelimit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/database"
)

// FailedQueue manages failed items and rate limit state persistence
type FailedQueue struct {
	db *database.DB
}

func NewFailedQueue(db *database.DB) *FailedQueue {
	return &FailedQueue{db: db}
}

// GetState retrieves the rate limit state for a specific kind
func (fq *FailedQueue) GetState(ctx context.Context, kind string) (*database.RateLimitState, error) {
	state, err := fq.db.GetRateLimitState(kind)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit state for %s: %w", kind, err)
	}

	if state == nil {
		// Return a default state if none exists
		return &database.RateLimitState{
			Kind:       kind,
			IsSleeping: false,
		}, nil
	}

	return state, nil
}

// SetState atomically updates the rate limit state
func (fq *FailedQueue) SetState(ctx context.Context, kind string, isSleeping bool, resumeAt *time.Time, reason string) error {
	state := database.RateLimitState{
		Kind:       kind,
		IsSleeping: isSleeping,
		ResumeAt:   resumeAt,
		Reason:     reason,
	}

	if err := fq.db.SetRateLimitState(state); err != nil {
		return fmt.Errorf("failed to set rate limit state for %s: %w", kind, err)
	}

	if isSleeping {
		log.Printf("Rate limit state updated: kind=%s, sleeping=true, resumeAt=%s, reason=%s",
			kind, resumeAt.Format("2006-01-02 15:04:05 MST"), reason)
	} else {
		log.Printf("Rate limit state updated: kind=%s, sleeping=false", kind)
	}

	return nil
}

// Enqueue adds a failed item to the queue
func (fq *FailedQueue) Enqueue(ctx context.Context, kind string, payload interface{}, reason string) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	item := database.FailedItem{
		Kind:      kind,
		Payload:   string(payloadJSON),
		Reason:    reason,
		CreatedAt: time.Now(),
	}

	if err := fq.db.EnqueueFailedItem(item); err != nil {
		return fmt.Errorf("failed to enqueue failed item: %w", err)
	}

	log.Printf("Enqueued failed item: kind=%s, reason=%s", kind, reason)
	return nil
}

// DequeueBatch retrieves a batch of failed items
func (fq *FailedQueue) DequeueBatch(ctx context.Context, kind string, limit int) ([]database.FailedItem, error) {
	items, err := fq.db.DequeueFailedItems(kind, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue items: %w", err)
	}

	if len(items) > 0 {
		log.Printf("Dequeued %d failed items of kind %s", len(items), kind)
	}

	return items, nil
}

// DeleteItem removes a successfully processed item
func (fq *FailedQueue) DeleteItem(ctx context.Context, id int) error {
	if err := fq.db.DeleteFailedItem(id); err != nil {
		return fmt.Errorf("failed to delete item %d: %w", id, err)
	}

	log.Printf("Deleted successfully processed failed item: id=%d", id)
	return nil
}

// ClearAll removes all failed items of a specific kind
func (fq *FailedQueue) ClearAll(ctx context.Context, kind string) error {
	if err := fq.db.ClearFailedItems(kind); err != nil {
		return fmt.Errorf("failed to clear items for kind %s: %w", kind, err)
	}

	log.Printf("Cleared all failed items for kind %s", kind)
	return nil
}

// WaitUntilResumeTime waits until the resume time if currently sleeping
func (fq *FailedQueue) WaitUntilResumeTime(ctx context.Context, kind string) error {
	state, err := fq.GetState(ctx, kind)
	if err != nil {
		return err
	}

	if !state.IsSleeping {
		log.Printf("Kind %s is not sleeping, no wait needed", kind)
		return nil
	}

	if state.ResumeAt == nil {
		log.Printf("Warning: kind %s is sleeping but has no resume time, clearing sleep state", kind)
		return fq.SetState(ctx, kind, false, nil, "")
	}

	log.Printf("Kind %s is sleeping, waiting until resume time: %s",
		kind, state.ResumeAt.Format("2006-01-02 15:04:05 MST"))

	if err := SleepUntilCtx(ctx, *state.ResumeAt); err != nil {
		return err
	}

	// Clear sleeping state after waking up
	if err := fq.SetState(ctx, kind, false, nil, ""); err != nil {
		return fmt.Errorf("failed to clear sleeping state after wake: %w", err)
	}

	log.Printf("Kind %s resumed after rate limit wait", kind)
	return nil
}
