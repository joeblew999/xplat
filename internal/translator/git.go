package translator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitManager handles Git operations for tracking translations
type GitManager struct {
	repoPath string
}

// NewGitManager creates a new Git manager
func NewGitManager() (*GitManager, error) {
	// Get current working directory (should be repo root)
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Verify this is a Git repository
	if _, err := os.Stat(filepath.Join(cwd, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a Git repository: %s", cwd)
	}

	return &GitManager{
		repoPath: cwd,
	}, nil
}

// GetChangedFiles returns a list of files that have changed since the checkpoint
func (g *GitManager) GetChangedFiles(checkpointTag, pathFilter string) ([]string, error) {
	// Check if checkpoint tag exists
	tagExists, err := g.tagExists(checkpointTag)
	if err != nil {
		return nil, err
	}

	var compareRef string
	if tagExists {
		compareRef = checkpointTag
	} else {
		// If no checkpoint tag exists, compare with initial commit
		// This means all files will be considered "changed"
		compareRef = g.getInitialCommit()
		if compareRef == "" {
			// No commits yet, return all files in directory
			return g.getAllFilesInPath(pathFilter)
		}
	}

	// Get list of changed files
	cmd := exec.Command("git", "diff", "--name-only", compareRef, "HEAD", "--", pathFilter)
	cmd.Dir = g.repoPath

	output, err := cmd.Output()
	if err != nil {
		// If git diff fails, try to list all files
		return g.getAllFilesInPath(pathFilter)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var changedFiles []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Only include markdown files
		if strings.HasSuffix(line, ".md") {
			fullPath := filepath.Join(g.repoPath, line)
			if _, err := os.Stat(fullPath); err == nil {
				changedFiles = append(changedFiles, fullPath)
			}
		}
	}

	return changedFiles, nil
}

// UpdateCheckpoint updates the translation checkpoint tag
func (g *GitManager) UpdateCheckpoint(checkpointTag string, translatedFiles []string) error {
	if len(translatedFiles) == 0 {
		return nil
	}

	// Create a commit with translated files
	fileList := make([]string, len(translatedFiles))
	for i, f := range translatedFiles {
		fileList[i] = filepath.Base(f)
	}

	commitMsg := fmt.Sprintf("Translate: %s\n\nAuto-translated from English source files", strings.Join(fileList, ", "))

	// Add all content directories (to include translated files)
	cmd := exec.Command("git", "add", "content/")
	cmd.Dir = g.repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to git add: %w", err)
	}

	// Create commit
	cmd = exec.Command("git", "commit", "-m", commitMsg)
	cmd.Dir = g.repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		// It's okay if there's nothing to commit
		if !strings.Contains(string(output), "nothing to commit") {
			return fmt.Errorf("failed to git commit: %w\n%s", err, output)
		}
	}

	// Update or create tag
	tagExists, err := g.tagExists(checkpointTag)
	if err != nil {
		return err
	}

	if tagExists {
		// Delete old tag
		cmd = exec.Command("git", "tag", "-d", checkpointTag)
		cmd.Dir = g.repoPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to delete old tag: %w", err)
		}
	}

	// Create new tag
	cmd = exec.Command("git", "tag", checkpointTag)
	cmd.Dir = g.repoPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}

	fmt.Printf("✅ Updated checkpoint tag: %s\n", checkpointTag)
	return nil
}

// tagExists checks if a Git tag exists
func (g *GitManager) tagExists(tag string) (bool, error) {
	cmd := exec.Command("git", "tag", "-l", tag)
	cmd.Dir = g.repoPath

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check tag: %w", err)
	}

	return strings.TrimSpace(string(output)) != "", nil
}

// getInitialCommit returns the hash of the initial commit
func (g *GitManager) getInitialCommit() string {
	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = g.repoPath

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// getAllFilesInPath returns all markdown files in a directory
func (g *GitManager) getAllFilesInPath(pathFilter string) ([]string, error) {
	var files []string

	fullPath := filepath.Join(g.repoPath, pathFilter)
	err := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}
