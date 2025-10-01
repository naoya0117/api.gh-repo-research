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
	
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("@google/gemini-cli not available: %w", err)
	}
	
	if !strings.Contains(string(output), "gemini-cli") {
		return fmt.Errorf("unexpected gemini-cli output")
	}
	
	testCmd := exec.Command("npx", "-y", "@google/gemini-cli", "--prompt", "Hello")
	_, err = testCmd.Output()
	if err != nil {
		return fmt.Errorf("gemini-cli authentication failed. Please run 'npx -y @google/gemini-cli auth' first: %w", err)
	}
	
	return nil
}

func (gc *GeminiClient) AnalyzeRepository(repoPath string, query database.CheckQuery) (string, error) {
	promptFile, err := gc.createPromptFile(repoPath, query)
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

func (gc *GeminiClient) createPromptFile(repoPath string, query database.CheckQuery) (string, error) {
	repoContent, err := gc.collectRepositoryContent(repoPath)
	if err != nil {
		return "", err
	}
	
	description := ""
	if query.Description != nil {
		description = *query.Description
	}
	
	prompt := fmt.Sprintf(`チェック項目: %s
説明: %s

質問: %s

対象リポジトリの構造と主要ファイル:
%s

上記の情報に基づいて分析してください。
`, query.Name, description, query.QueryTemplate, repoContent)
	
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

func (gc *GeminiClient) collectRepositoryContent(repoPath string) (string, error) {
	var content strings.Builder
	
	content.WriteString("## ディレクトリ構造\n")
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || strings.Contains(path, ".git") {
			return nil
		}
		
		relPath, _ := filepath.Rel(repoPath, path)
		if relPath == "." {
			return nil
		}
		
		if info.IsDir() {
			content.WriteString(fmt.Sprintf("%s/\n", relPath))
		} else if gc.isRelevantFile(relPath) {
			content.WriteString(fmt.Sprintf("%s\n", relPath))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	
	content.WriteString("\n## 主要ファイル内容\n")
	importantFiles := []string{
		"README.md", "readme.md", "README.txt",
		"package.json", "go.mod", "Cargo.toml", "pom.xml",
		"Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		"Makefile", "CMakeLists.txt",
		".github/workflows/*.yml", ".github/workflows/*.yaml",
	}
	
	for _, pattern := range importantFiles {
		matches, _ := filepath.Glob(filepath.Join(repoPath, pattern))
		for _, match := range matches {
			if fileContent, err := gc.readFileContent(match, 1000); err == nil {
				relPath, _ := filepath.Rel(repoPath, match)
				content.WriteString(fmt.Sprintf("\n### %s\n```\n%s\n```\n", 
					relPath, fileContent))
			}
		}
	}
	
	return content.String(), nil
}

func (gc *GeminiClient) isRelevantFile(filename string) bool {
	relevantExts := []string{
		".md", ".txt", ".json", ".yml", ".yaml", 
		".go", ".js", ".ts", ".py", ".java", ".rs", ".c", ".cpp", ".h",
		"Dockerfile", "Makefile", "LICENSE", ".gitignore",
	}
	
	filename = strings.ToLower(filename)
	for _, ext := range relevantExts {
		if strings.HasSuffix(filename, strings.ToLower(ext)) || 
		   strings.Contains(filename, strings.ToLower(ext)) {
			return true
		}
	}
	return false
}

func (gc *GeminiClient) readFileContent(filePath string, maxLines int) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	
	if len(content) > 10000 {
		content = content[:10000]
	}
	
	lines := strings.Split(string(content), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "... (truncated)")
	}
	
	return strings.Join(lines, "\n"), nil
}