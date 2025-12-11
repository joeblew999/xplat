// Package translator provides translation workflow management.
//
// This file contains the Checker type - the main entry point for translation operations.
// Query functions are in checker.go, mutations in mutator.go, output in presenter.go.
package translator

import (
	"fmt"
	"path/filepath"
)

// Checker handles translation status checking and management operations.
// It provides methods for:
//   - Querying translation status (CheckStatus, CheckMissing, etc.) - see checker.go
//   - Performing mutations (DoClean, DoDone, etc.) - see mutator.go
type Checker struct {
	config *Config
	git    *GitManager
}

// NewChecker creates a new Checker instance.
// No API key is needed - this is for status and management commands only.
func NewChecker() (*Checker, error) {
	config := DefaultConfig()

	git, err := NewGitManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create Git manager: %w", err)
	}

	return &Checker{
		config: config,
		git:    git,
	}, nil
}

// GetConfig returns the translator configuration.
func (c *Checker) GetConfig() *Config {
	return c.config
}

// sourcePath returns the full path to the source content directory (e.g., "content/english").
func (c *Checker) sourcePath() string {
	return filepath.Join(c.config.ContentDir, c.config.SourceDir)
}
