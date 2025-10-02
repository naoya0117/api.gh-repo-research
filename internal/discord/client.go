package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Message struct {
	Content string `json:"content"`
}

type Client struct {
	webhookURL string
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		webhookURL: os.Getenv("DISCORD_WEBHOOK_URL"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) SendMessage(message string) error {
	if c.webhookURL == "" {
		return fmt.Errorf("DISCORD_WEBHOOK_URL environment variable is not set")
	}

	payload := Message{
		Content: message,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequest("POST", c.webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord API returned status %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) SendCompletionNotification(command string, duration time.Duration) error {
	message := fmt.Sprintf("🎉 CLI実行完了\n\nコマンド: `%s`\n実行時間: %v\n完了時刻: %s", 
		command, 
		duration.Round(time.Second), 
		time.Now().Format("2006-01-02 15:04:05"))
	
	return c.SendMessage(message)
}