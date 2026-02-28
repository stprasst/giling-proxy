package services

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GitHelper handles git operations
type GitHelper struct {
	enabled bool
	repoDir string
}

// NewGitHelper creates a new git helper
func NewGitHelper(repoDir string, enabled bool) *GitHelper {
	return &GitHelper{
		enabled: enabled,
		repoDir: repoDir,
	}
}

// CommitAndPush commits and pushes changes to git
func (g *GitHelper) CommitAndPush(message string) error {
	if !g.enabled {
		return nil // Disabled, skip
	}

	// Check if we're in a git repo
	if !g.isGitRepo() {
		return fmt.Errorf("not a git repository")
	}

	// Check if there are changes
	if !g.hasChanges() {
		return nil // Nothing to commit
	}

	// Add all changes in data/public directory
	if err := g.gitAdd("data/public/"); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Commit with message
	commitMsg := fmt.Sprintf("%s - %s", message, time.Now().Format("2006-01-02 15:04:05"))
	if err := g.gitCommit(commitMsg); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Push to origin
	if err := g.gitPush(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
}

// isGitRepo checks if current directory is a git repository
func (g *GitHelper) isGitRepo() bool {
	cmd := exec.Command("git", "-C", g.repoDir, "rev-parse", "--git-dir")
	err := cmd.Run()
	return err == nil
}

// hasChanges checks if there are uncommitted changes
func (g *GitHelper) hasChanges() bool {
	cmd := exec.Command("git", "-C", g.repoDir, "status", "--porcelain", "data/public/")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) != ""
}

// gitAdd stages files for commit
func (g *GitHelper) gitAdd(path string) error {
	cmd := exec.Command("git", "-C", g.repoDir, "add", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// gitCommit creates a commit
func (g *GitHelper) gitCommit(message string) error {
	cmd := exec.Command("git", "-C", g.repoDir, "commit", "-m", message)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// gitPush pushes commits to remote
func (g *GitHelper) gitPush() error {
	cmd := exec.Command("git", "-C", g.repoDir, "push")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}
