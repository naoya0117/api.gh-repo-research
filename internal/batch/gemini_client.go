package batch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/naoya0117/shuron2025/api/internal/database"
	"github.com/naoya0117/shuron2025/api/internal/ratelimit"
)

type GeminiClient struct {
	workingDir string
	timeout    time.Duration
}

type GeminiRateLimitError struct {
	message   string
	resetTime *time.Time
}

func (e *GeminiRateLimitError) Error() string {
	if e.resetTime != nil {
		return fmt.Sprintf("gemini rate limit: %s (resets at %s)", e.message, e.resetTime.Format("2006-01-02 15:04:05 MST"))
	}
	return fmt.Sprintf("gemini rate limit: %s", e.message)
}

func (e *GeminiRateLimitError) Is(target error) bool {
	return target == ratelimit.ErrRateLimited
}

func NewGeminiRateLimitError(message string, resetTime *time.Time) *GeminiRateLimitError {
	return &GeminiRateLimitError{
		message:   message,
		resetTime: resetTime,
	}
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

	testCmd := exec.Command("npx", "-y", "@google/gemini-cli", "--allowed-tools", "read_file,list_directory,glob,search_file_content,web_fetch,google_web_search", "-p", "Hello")
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

	// プロンプトファイルの内容を読み込み
	promptContent, err := os.ReadFile(promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file: %w", err)
	}
	
	cmd := exec.CommandContext(ctx, "npx", "-y", "@google/gemini-cli", "--include-directories", repoPath, "--allowed-tools", "read_file,list_directory,glob,search_file_content,web_fetch,google_web_search", "-p", string(promptContent))
	cmd.Dir = repoPath
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	err = cmd.Run()

	// Check stdout first - if there's valid output, consider it a success
	// even if the process was killed (e.g., due to timeout after writing results)
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if err != nil {
		// If stdout contains a valid analysis result, treat as success despite the error
		if gc.isValidAnalysisResult(stdoutStr) {
			log.Printf("Gemini CLI completed with error (%v) but produced valid output, treating as success", err)
			if stderr.Len() > 0 {
				log.Printf("Gemini CLI warning - stderr: %s", stderrStr)
			}
			return stdoutStr, nil
		}

		errorMsg := fmt.Sprintf("exit error: %v, stderr: %s, stdout: %s", err, stderrStr, stdoutStr)

		// ログにはエラー詳細を出力
		log.Printf("Gemini CLI error - stderr: %s", stderrStr)
		log.Printf("Gemini CLI error - stdout: %s", stdoutStr)

		// Check for rate limit and try to extract reset time
		if gc.isRateLimitError(err) || gc.isRateLimitError(fmt.Errorf("%s", stderrStr)) {
			resetTime := gc.parseGeminiResetTime(stderrStr + " " + stdoutStr)
			return "", NewGeminiRateLimitError(fmt.Sprintf("gemini-cli rate limit exceeded: %s", errorMsg), resetTime)
		}
		return "", fmt.Errorf("gemini-cli execution failed: %s", errorMsg)
	}

	// 正常時は標準出力のみを返す
	if stderr.Len() > 0 {
		log.Printf("Gemini CLI warning - stderr: %s", stderrStr)
	}

	// Validate output even on success to avoid saving empty/invalid results
	if !gc.isValidAnalysisResult(stdoutStr) {
		return "", fmt.Errorf("gemini-cli produced invalid or empty output: %q", stdoutStr)
	}

	return stdoutStr, nil
}

func (gc *GeminiClient) AnalyzeRepositoryWithRetry(repoPath string, query database.CheckQuery, maxRetries int) (string, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		result, err := gc.AnalyzeRepository(repoPath, query)
		if err == nil {
			return result, nil
		}

		// Don't retry on rate limit errors - propagate immediately
		if errors.Is(err, ratelimit.ErrRateLimited) {
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
		"quota limit",
		"too many requests",
		"429",
		"resource_exhausted",
		"daily quota",
		"gemini-2.5-pro quota",
	}

	for _, keyword := range rateLimitKeywords {
		if strings.Contains(errorStr, keyword) {
			return true
		}
	}

	return false
}

// isValidAnalysisResult checks if the output contains a valid analysis result
func (gc *GeminiClient) isValidAnalysisResult(output string) bool {
	if len(strings.TrimSpace(output)) == 0 {
		return false
	}

	// Check for expected output format markers
	requiredMarkers := []string{
		"評価結果",
		"判断理由",
	}

	lowerOutput := strings.ToLower(output)
	for _, marker := range requiredMarkers {
		if !strings.Contains(lowerOutput, strings.ToLower(marker)) {
			return false
		}
	}

	return true
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

func (gc *GeminiClient) parseGeminiResetTime(output string) *time.Time {
	// Try to extract reset time from Gemini CLI error messages
	// Common patterns:
	// - "try again in X seconds"
	// - "retry after HH:MM:SS"
	// - "reset at YYYY-MM-DDTHH:MM:SSZ"

	// Pattern 1: "try again in X seconds/minutes/hours"
	reSeconds := regexp.MustCompile(`try again in (\d+)\s*(second|minute|hour)s?`)
	if matches := reSeconds.FindStringSubmatch(output); len(matches) >= 3 {
		var duration time.Duration
		amount := 0
		fmt.Sscanf(matches[1], "%d", &amount)

		switch matches[2] {
		case "second":
			duration = time.Duration(amount) * time.Second
		case "minute":
			duration = time.Duration(amount) * time.Minute
		case "hour":
			duration = time.Duration(amount) * time.Hour
		}

		if duration > 0 {
			resetTime := time.Now().Add(duration)
			log.Printf("Parsed Gemini reset time from error: %s", resetTime.Format("2006-01-02 15:04:05 MST"))
			return &resetTime
		}
	}

	// Pattern 2: ISO 8601 timestamp
	reISO := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2}))`)
	if matches := reISO.FindStringSubmatch(output); len(matches) >= 2 {
		if t, err := time.Parse(time.RFC3339, matches[1]); err == nil {
			log.Printf("Parsed Gemini reset time from ISO format: %s", t.Format("2006-01-02 15:04:05 MST"))
			return &t
		}
	}

	log.Printf("Could not parse reset time from Gemini error, will use JST midnight fallback")
	return nil
}

func IsGeminiRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ratelimit.ErrRateLimited)
}
