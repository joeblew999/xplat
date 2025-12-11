// Package translator provides translation workflow management.
//
// This file contains mutation functions that modify state (filesystem, git).
// These functions return result types (defined in results.go) that are then
// passed to presenters for output formatting.
package translator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ============================================================================
// Mutation Functions - Side effects (writes to filesystem, git)
// ============================================================================

// DoClean deletes orphaned files in target languages.
// If force is false, it only computes what would be deleted (for confirmation).
// If force is true (or after confirmation), it performs deletion.
func (c *Checker) DoClean(force bool, confirmed bool) CleanResult {
	result := CleanResult{
		FilesToDelete: make(map[string][]string),
	}

	// Find orphans per language
	for _, lang := range c.config.TargetLangs {
		langPath := filepath.Join(c.config.ContentDir, lang.DirName)
		filepath.Walk(langPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			relPath := strings.TrimPrefix(path, langPath+string(os.PathSeparator))
			enFile := filepath.Join(c.sourcePath(), relPath)
			if _, err := os.Stat(enFile); os.IsNotExist(err) {
				result.FilesToDelete[lang.Name] = append(result.FilesToDelete[lang.Name], path)
				result.TotalCount++
			}
			return nil
		})
	}

	// If no orphans or not confirmed, just return the list
	if result.TotalCount == 0 || (!force && !confirmed) {
		return result
	}

	// Delete files
	for _, files := range result.FilesToDelete {
		for _, path := range files {
			if err := os.Remove(path); err != nil {
				result.Error = fmt.Errorf("error deleting %s: %w", path, err)
				return result
			}
		}
	}

	result.Deleted = true
	return result
}

// DoDone updates the checkpoint tag to current commit.
func (c *Checker) DoDone() DoneResult {
	result := DoneResult{
		NewTag: c.config.CheckpointTag,
	}

	// Get old tag SHA if it exists
	oldCmd := exec.Command("git", "rev-parse", c.config.CheckpointTag)
	if oldOut, err := oldCmd.Output(); err == nil {
		result.OldTag = strings.TrimSpace(string(oldOut))
	}

	// Update tag
	cmd := exec.Command("git", "tag", "-f", c.config.CheckpointTag, "HEAD")
	if err := cmd.Run(); err != nil {
		result.Error = err
		return result
	}

	// Get new commit SHA
	commitCmd := exec.Command("git", "rev-parse", "HEAD")
	if commitOut, err := commitCmd.Output(); err == nil {
		result.Commit = strings.TrimSpace(string(commitOut))[:7] // Short SHA
	}

	return result
}

// DoMenuSync regenerates translated menu files from English.
func (c *Checker) DoMenuSync() MenuSyncResult {
	var result MenuSyncResult

	// Parse English menu (source of truth)
	enMenuPath := GetMenuFilePath("en")
	enMenu, err := ParseMenuFile(enMenuPath)
	if err != nil {
		result.Error = fmt.Errorf("error reading English menu: %w", err)
		return result
	}

	// Generate menu for each target language
	for _, lang := range c.config.TargetLangs {
		menuPath := GetMenuFilePath(lang.Code)
		content := GenerateMenuFile(enMenu, lang.Code)

		err := os.WriteFile(menuPath, []byte(content), 0644)
		if err != nil {
			result.Error = fmt.Errorf("error writing %s: %w", menuPath, err)
			return result
		}
		result.FilesWritten = append(result.FilesWritten, menuPath)
	}

	return result
}

// DoLangAdd adds a new language to Hugo config and creates content directory.
func (c *Checker) DoLangAdd(code, name, dirname string) LangAddResult {
	result := LangAddResult{
		Code:    code,
		Name:    name,
		DirName: dirname,
	}

	// Check if language already exists
	existing, err := GetLanguageByCode(code)
	if err != nil {
		result.Error = fmt.Errorf("error reading config: %w", err)
		return result
	}
	if existing != nil {
		result.Error = fmt.Errorf("language '%s' already exists in config", code)
		return result
	}

	// Add to languages.toml
	result.ConfigPath = "config/_default/languages.toml"
	if err := AddLanguageToHugo(code, name, dirname); err != nil {
		result.Error = fmt.Errorf("error updating languages.toml: %w", err)
		return result
	}

	// Create content directory
	result.ContentPath = filepath.Join(c.config.ContentDir, dirname)
	if err := CreateContentDirectory(dirname, name); err != nil {
		result.Error = fmt.Errorf("error creating directory: %w", err)
		return result
	}

	// Generate menu file
	enMenuPath := GetMenuFilePath("en")
	enMenu, err := ParseMenuFile(enMenuPath)
	if err == nil {
		menuPath := GetMenuFilePath(code)
		content := GenerateMenuFile(enMenu, code)
		if err := os.WriteFile(menuPath, []byte(content), 0644); err == nil {
			result.MenuPath = menuPath
		}
	}

	return result
}

// DoLangRemove removes a language from Hugo config and deletes content.
// If force is false and there are files, it returns without deleting for confirmation.
func (c *Checker) DoLangRemove(code string, force bool, confirmed bool) LangRemoveResult {
	result := LangRemoveResult{
		Code: code,
	}

	// Check if language exists
	existing, err := GetLanguageByCode(code)
	if err != nil {
		result.Error = fmt.Errorf("error reading config: %w", err)
		return result
	}
	if existing == nil {
		result.Error = fmt.Errorf("language '%s' not found in config", code)
		return result
	}

	// Get content directory
	dirname := strings.TrimPrefix(existing.ContentDir, "content/")
	contentPath := filepath.Join(c.config.ContentDir, dirname)

	// Count files in content directory
	if _, err := os.Stat(contentPath); err == nil {
		filepath.Walk(contentPath, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
				result.FilesRemoved++
			}
			return nil
		})
	}

	// If has files and not force/confirmed, return for confirmation
	if result.FilesRemoved > 0 && !force && !confirmed {
		return result
	}

	// If user cancelled (not force, not confirmed, but files exist)
	// This case is handled by the caller who asks for confirmation

	// Remove from languages.toml
	if err := RemoveLanguageFromHugo(code); err != nil {
		result.Error = fmt.Errorf("error updating languages.toml: %w", err)
		return result
	}

	// Delete content directory
	if _, err := os.Stat(contentPath); err == nil {
		if err := os.RemoveAll(contentPath); err != nil {
			result.Error = fmt.Errorf("error deleting directory: %w", err)
			return result
		}
	}

	// Delete menu file
	menuPath := GetMenuFilePath(code)
	if _, err := os.Stat(menuPath); err == nil {
		os.Remove(menuPath) // Ignore error - menu is optional
	}

	return result
}

// DoLangInit initializes content directory for a configured language.
func (c *Checker) DoLangInit(code string) LangInitResult {
	result := LangInitResult{
		Code: code,
	}

	// Check if language exists in config
	existing, err := GetLanguageByCode(code)
	if err != nil {
		result.Error = fmt.Errorf("error reading config: %w", err)
		return result
	}
	if existing == nil {
		result.Error = fmt.Errorf("language '%s' not found in config", code)
		return result
	}

	// Get content directory
	result.DirName = strings.TrimPrefix(existing.ContentDir, "content/")
	result.Path = filepath.Join(c.config.ContentDir, result.DirName)

	// Check if directory already exists
	if _, err := os.Stat(result.Path); err == nil {
		result.AlreadyExists = true
		// Count files
		filepath.Walk(result.Path, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
				result.FileCount++
			}
			return nil
		})
		return result
	}

	// Create content directory
	if err := CreateContentDirectory(result.DirName, existing.LanguageName); err != nil {
		result.Error = fmt.Errorf("error creating directory: %w", err)
		return result
	}

	return result
}
