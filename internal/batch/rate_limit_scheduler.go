package batch

import (
	"fmt"
	"log"
	"time"
)

type RateLimitScheduler struct {
	rateLimitResetTime *time.Time
}

func NewRateLimitScheduler() *RateLimitScheduler {
	return &RateLimitScheduler{}
}

func (rls *RateLimitScheduler) HandleRateLimit() error {
	now := time.Now()
	
	nextMidnight := rls.GetNextMidnight(now)
	rls.rateLimitResetTime = &nextMidnight
	
	duration := nextMidnight.Sub(now)
	log.Printf("Rate limit detected. Waiting until next midnight (%s) - Duration: %v", 
		nextMidnight.Format("2006-01-02 15:04:05"), duration)
	
	if duration > 24*time.Hour {
		return fmt.Errorf("calculated wait duration is too long: %v", duration)
	}
	
	if duration < 0 {
		log.Printf("Warning: calculated duration is negative, using 1 minute instead")
		duration = 1 * time.Minute
	}
	
	rls.waitWithProgress(duration, nextMidnight)
	
	rls.rateLimitResetTime = nil
	log.Printf("Rate limit wait period completed. Resuming operations at %s", 
		time.Now().Format("2006-01-02 15:04:05"))
	
	return nil
}

func (rls *RateLimitScheduler) GetNextMidnight(now time.Time) time.Time {
	nextDay := now.AddDate(0, 0, 1)
	
	return time.Date(
		nextDay.Year(),
		nextDay.Month(),
		nextDay.Day(),
		0, 0, 0, 0,
		now.Location(),
	)
}

func (rls *RateLimitScheduler) waitWithProgress(duration time.Duration, targetTime time.Time) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	
	timer := time.NewTimer(duration)
	defer timer.Stop()
	
	log.Printf("Starting rate limit wait. Target time: %s", targetTime.Format("2006-01-02 15:04:05"))
	
	for {
		select {
		case <-timer.C:
			return
			
		case <-ticker.C:
			remaining := targetTime.Sub(time.Now())
			if remaining <= 0 {
				return
			}
			
			log.Printf("Rate limit wait in progress. Time remaining: %v (Target: %s)", 
				remaining.Round(time.Minute), targetTime.Format("2006-01-02 15:04:05"))
		}
	}
}

func (rls *RateLimitScheduler) IsWaitingForRateLimit() bool {
	if rls.rateLimitResetTime == nil {
		return false
	}
	
	return time.Now().Before(*rls.rateLimitResetTime)
}

func (rls *RateLimitScheduler) GetRateLimitResetTime() *time.Time {
	return rls.rateLimitResetTime
}

func (rls *RateLimitScheduler) GetRemainingWaitTime() time.Duration {
	if rls.rateLimitResetTime == nil {
		return 0
	}
	
	remaining := rls.rateLimitResetTime.Sub(time.Now())
	if remaining < 0 {
		return 0
	}
	
	return remaining
}