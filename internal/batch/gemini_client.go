package batch

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/database"
)

type GeminiClient struct {
	workingDir string
	timeout    time.Duration
}

type RateLimitError struct {
	message string
}

func (e *RateLimitError) Error() string {
	return e.message
}

func NewRateLimitError(message string) *RateLimitError {
	return &RateLimitError{message: message}
}

func NewGeminiClient(workingDir string, timeout time.Duration) *GeminiClient {
	return &GeminiClient{
		workingDir: workingDir,
		timeout:    timeout,
	}
}

func (gc *GeminiClient) CheckAuthentication() error {
	cmd := exec.Command("npx", "-y", "@google/gemini-cli", "--help")

	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("@google/gemini-cli not available: %w", err)
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("gemini-cli コマンドの終了コードが異常です: %v", exitErr)
	}

	testCmd := exec.Command("npx", "-y", "@google/gemini-cli", "--prompt", "Hello")
	_, err = testCmd.Output()
	if err != nil {
		return fmt.Errorf("gemini-cli authentication failed. Please run 'npx -y @google/gemini-cli auth' first: %w", err)
	}

	return nil
}

func (gc *GeminiClient) AnalyzeRepository(repoPath string, query database.CheckQuery) (string, error) {
	promptFile, err := gc.createPromptFile(query)
	if err != nil {
		return "", err
	}
	defer os.Remove(promptFile)

	ctx, cancel := context.WithTimeout(context.Background(), gc.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npx", "-y", "@google/gemini-cli", "--prompt-file", promptFile)
	cmd.Dir = repoPath
	cmd.Env = os.Environ()

	output, err := cmd.Output()
	if err != nil {
		if gc.isRateLimitError(err) {
			return "", NewRateLimitError(fmt.Sprintf("gemini-cli rate limit exceeded: %v", err))
		}
		return "", fmt.Errorf("gemini-cli execution failed: %w", err)
	}

	return string(output), nil
}

func (gc *GeminiClient) AnalyzeRepositoryWithRetry(repoPath string, query database.CheckQuery, maxRetries int) (string, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		result, err := gc.AnalyzeRepository(repoPath, query)
		if err == nil {
			return result, nil
		}

		if _, isRateLimit := err.(*RateLimitError); isRateLimit {
			return "", err
		}

		lastErr = err
		log.Printf("Retry %d/%d: gemini-cli failed for repo %s, query %s: %v",
			i+1, maxRetries, filepath.Base(repoPath), query.Name, err)

		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}

	return "", fmt.Errorf("gemini-cli failed after %d retries: %w", maxRetries, lastErr)
}

func (gc *GeminiClient) isRateLimitError(err error) bool {
	errorStr := strings.ToLower(err.Error())
	rateLimitKeywords := []string{
		"rate limit",
		"quota exceeded",
		"too many requests",
		"429",
		"resource_exhausted",
	}

	for _, keyword := range rateLimitKeywords {
		if strings.Contains(errorStr, keyword) {
			return true
		}
	}

	return false
}

func (gc *GeminiClient) createPromptFile(query database.CheckQuery) (string, error) {
	description := ""
	if query.Description != nil {
		description = *query.Description
	}

	prompt := fmt.Sprintf(`あなたは経験豊富なソフトウェアエンジニアです。このGitリポジトリを分析してください。

【分析項目】
%s

【詳細説明】
%s

【分析指示】
%s

【出力形式】
以下の形式で回答してください：
- 評価結果: 適当/一部適当/不適当[○/△/×](分析の指示に従う)
- 参考になる箇所: ファイル名-行番号
- 判断理由: 十分な根拠を示す
- 主要な発見事項: (箇条書きで3-5点)
- 推奨される改善点: (具体的な提案)

リポジトリ全体を確認し、具体的で実用的な分析を提供してください。
`, query.Name, description, query.QueryTemplate)

	tmpFile, err := os.CreateTemp("", "gemini_prompt_*.txt")
	if err != nil {
		return "", err
	}

	_, err = tmpFile.WriteString(prompt)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", err
	}

	tmpFile.Close()
	return tmpFile.Name(), nil
}
