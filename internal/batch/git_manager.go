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
)

type GitManager struct {
	baseDir string
	timeout time.Duration
}

func NewGitManager(baseDir string, timeout time.Duration) *GitManager {
	return &GitManager{
		baseDir: baseDir,
		timeout: timeout,
	}
}

func (gm *GitManager) Clone(repoURL, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	if _, err := os.Stat(destPath); err == nil {
		log.Printf("Directory %s already exists, removing it", destPath)
		if err := os.RemoveAll(destPath); err != nil {
			return fmt.Errorf("failed to remove existing directory: %w", err)
		}
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), gm.timeout)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, destPath)
	cmd.Env = os.Environ()
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed for %s: %w\nOutput: %s", repoURL, err, string(output))
	}
	
	log.Printf("Successfully cloned %s to %s", repoURL, destPath)
	return nil
}

func (gm *GitManager) Update(repoPath string) error {
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository: %s", repoPath)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), gm.timeout)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, "git", "pull", "origin")
	cmd.Dir = repoPath
	cmd.Env = os.Environ()
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w\nOutput: %s", err, string(output))
	}
	
	log.Printf("Successfully updated repository at %s", repoPath)
	return nil
}

func (gm *GitManager) Cleanup(repoPath string) error {
	if repoPath == "" || repoPath == "/" || repoPath == gm.baseDir {
		return fmt.Errorf("refusing to remove dangerous path: %s", repoPath)
	}
	
	if !strings.HasPrefix(repoPath, gm.baseDir) {
		return fmt.Errorf("path %s is outside base directory %s", repoPath, gm.baseDir)
	}
	
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		log.Printf("Directory %s does not exist, skipping cleanup", repoPath)
		return nil
	}
	
	err := os.RemoveAll(repoPath)
	if err != nil {
		return fmt.Errorf("failed to remove directory %s: %w", repoPath, err)
	}
	
	log.Printf("Successfully cleaned up directory: %s", repoPath)
	return nil
}

func (gm *GitManager) GetRepositoryInfo(repoPath string) (*RepositoryInfo, error) {
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}
	
	info := &RepositoryInfo{}
	
	remoteURL, err := gm.getRemoteURL(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote URL: %w", err)
	}
	info.RemoteURL = remoteURL
	
	lastCommit, err := gm.getLastCommitHash(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get last commit: %w", err)
	}
	info.LastCommitHash = lastCommit
	
	branch, err := gm.getCurrentBranch(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	info.CurrentBranch = branch
	
	return info, nil
}

func (gm *GitManager) getRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(output)), nil
}

func (gm *GitManager) getLastCommitHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(output)), nil
}

func (gm *GitManager) getCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(output)), nil
}

func (gm *GitManager) ValidateRepository(repoPath string) error {
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repository path does not exist: %s", repoPath)
	}
	
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository: %s", repoPath)
	}
	
	info, err := os.Stat(repoPath)
	if err != nil {
		return fmt.Errorf("failed to get repository info: %w", err)
	}
	
	if !info.IsDir() {
		return fmt.Errorf("repository path is not a directory: %s", repoPath)
	}
	
	return nil
}

func (gm *GitManager) GetRepositorySize(repoPath string) (int64, error) {
	var size int64
	
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		
		if !info.IsDir() {
			size += info.Size()
		}
		
		return nil
	})
	
	if err != nil {
		return 0, fmt.Errorf("failed to calculate repository size: %w", err)
	}
	
	return size, nil
}

type RepositoryInfo struct {
	RemoteURL      string
	LastCommitHash string
	CurrentBranch  string
}