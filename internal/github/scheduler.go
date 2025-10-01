package github

import (
	"fmt"
	"time"
)

func WaitUntilMidnight() {
	now := time.Now()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	waitDuration := nextMidnight.Sub(now)
	
	fmt.Printf("⏰ Rate limit detected. Waiting until next midnight (%s)...\n", nextMidnight.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("⏳ Sleep duration: %v\n", waitDuration)
	
	time.Sleep(waitDuration)
	
	fmt.Printf("🌅 Midnight reached! Resuming collection at %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
}

func GetTimeUntilMidnight() time.Duration {
	now := time.Now()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	return nextMidnight.Sub(now)
}

func FormatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}