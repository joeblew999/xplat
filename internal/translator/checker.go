// Package translator provides translation workflow management.
//
// This file contains pure query functions that compute data without side effects.
// These functions return result types (defined in results.go) that are then
// passed to presenters for output formatting.
package translator

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ============================================================================
// Query Functions - Pure data computation, no printing, no side effects
// ============================================================================

// CheckStatus computes what English files changed since last translation.
func (c *Checker) CheckStatus() StatusResult {
	return StatusResult{
		NewFiles:           c.getNewFiles(),
		UncommittedChanges: c.getUncommittedChanges(),
		CommittedChanges:   c.getCommittedChanges(),
		CheckpointExists:   c.checkpointExists(),
		CheckpointTag:      c.config.CheckpointTag,
	}
}

// CheckDiff computes diff for a specific English file since last translation.
func (c *Checker) CheckDiff(file string) DiffResult {
	sourcePath := c.sourcePath()

	// Handle both formats: "blog/post.md" or "content/english/blog/post.md"
	var enFile, relPath string
	if strings.HasPrefix(file, sourcePath+"/") {
		enFile = file
		relPath = strings.TrimPrefix(file, sourcePath+"/")
	} else {
		enFile = filepath.Join(sourcePath, file)
		relPath = file
	}

	result := DiffResult{
		File: relPath,
	}

	if _, err := os.Stat(enFile); os.IsNotExist(err) {
		result.Error = err
		return result
	}

	// Check if file is new (not in last-translation)
	checkCmd := exec.Command("git", "show", c.config.CheckpointTag+":"+enFile)
	if err := checkCmd.Run(); err != nil {
		// File doesn't exist in checkpoint - it's new
		result.IsNew = true
		content, _ := os.ReadFile(enFile)
		result.DiffOutput = string(content)
		return result
	}

	// Show diff since last translation (committed changes)
	diffCmd := exec.Command("git", "diff", c.config.CheckpointTag+"..HEAD", "--", enFile)
	committedDiff, _ := diffCmd.Output()

	// Also check for uncommitted changes (working directory vs HEAD)
	uncommittedCmd := exec.Command("git", "diff", "HEAD", "--", enFile)
	uncommittedDiff, _ := uncommittedCmd.Output()

	// And check for staged but uncommitted
	stagedCmd := exec.Command("git", "diff", "--cached", "--", enFile)
	stagedDiff, _ := stagedCmd.Output()

	// Combine all diffs
	var combinedDiff strings.Builder
	if len(committedDiff) > 0 {
		combinedDiff.WriteString(string(committedDiff))
	}
	if len(stagedDiff) > 0 {
		combinedDiff.WriteString(string(stagedDiff))
	}
	if len(uncommittedDiff) > 0 {
		combinedDiff.WriteString(string(uncommittedDiff))
	}

	result.DiffOutput = combinedDiff.String()
	return result
}

// CheckMissing computes which languages are missing content files.
func (c *Checker) CheckMissing() MissingResult {
	englishFiles := c.getEnglishFiles()
	result := MissingResult{
		ByLanguage: make(map[string][]string),
	}

	for _, enFile := range englishFiles {
		relPath := strings.TrimPrefix(enFile, c.sourcePath()+string(os.PathSeparator))
		for _, lang := range c.config.TargetLangs {
			langFile := filepath.Join(c.config.ContentDir, lang.DirName, relPath)
			if _, err := os.Stat(langFile); os.IsNotExist(err) {
				result.ByLanguage[lang.Name] = append(result.ByLanguage[lang.Name], relPath)
				result.TotalCount++
			}
		}
	}

	return result
}

// CheckStale computes files that are smaller than English (may need updating).
func (c *Checker) CheckStale() StaleResult {
	englishFiles := c.getEnglishFiles()
	var result StaleResult

	for _, enFile := range englishFiles {
		enInfo, err := os.Stat(enFile)
		if err != nil {
			continue
		}
		enSize := enInfo.Size()
		if enSize <= 500 {
			continue // Skip small files
		}

		relPath := strings.TrimPrefix(enFile, c.sourcePath()+string(os.PathSeparator))
		threshold := enSize / 2

		for _, lang := range c.config.TargetLangs {
			langFile := filepath.Join(c.config.ContentDir, lang.DirName, relPath)
			langInfo, err := os.Stat(langFile)
			if err != nil {
				continue // File doesn't exist
			}
			if langInfo.Size() < threshold {
				result.Files = append(result.Files, StaleFile{
					SourcePath: enFile,
					TargetPath: langFile,
					LangCode:   lang.Code,
					SourceSize: enSize,
					TargetSize: langInfo.Size(),
					Ratio:      float64(langInfo.Size()) / float64(enSize),
				})
			}
		}
	}

	return result
}

// CheckOrphans computes files in target languages with no English source.
func (c *Checker) CheckOrphans() OrphansResult {
	result := OrphansResult{
		ByLanguage: make(map[string][]string),
	}

	for _, lang := range c.config.TargetLangs {
		langPath := filepath.Join(c.config.ContentDir, lang.DirName)
		filepath.Walk(langPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			// Convert to English path
			relPath := strings.TrimPrefix(path, langPath+string(os.PathSeparator))
			enFile := filepath.Join(c.sourcePath(), relPath)
			if _, err := os.Stat(enFile); os.IsNotExist(err) {
				result.ByLanguage[lang.Name] = append(result.ByLanguage[lang.Name], path)
				result.TotalCount++
			}
			return nil
		})
	}

	return result
}

// CheckNext computes the next file to translate with progress.
func (c *Checker) CheckNext() NextResult {
	englishFiles := c.getEnglishFiles()
	sort.Strings(englishFiles)

	result := NextResult{
		TotalFiles:   len(englishFiles),
		TranslatedBy: make(map[string]int),
		MissingBy:    make(map[string]int),
	}

	// Count missing per language
	for _, enFile := range englishFiles {
		relPath := strings.TrimPrefix(enFile, c.sourcePath()+string(os.PathSeparator))
		for _, lang := range c.config.TargetLangs {
			langFile := filepath.Join(c.config.ContentDir, lang.DirName, relPath)
			if _, err := os.Stat(langFile); os.IsNotExist(err) {
				result.MissingBy[lang.DirName]++
			} else {
				result.TranslatedBy[lang.DirName]++
			}
		}
	}

	// Check if all done
	totalMissing := 0
	for _, count := range result.MissingBy {
		totalMissing += count
	}
	if totalMissing == 0 {
		result.AllDone = true
		return result
	}

	// Find first missing file
	for _, enFile := range englishFiles {
		relPath := strings.TrimPrefix(enFile, c.sourcePath()+string(os.PathSeparator))
		var missingIn []string

		for _, lang := range c.config.TargetLangs {
			langFile := filepath.Join(c.config.ContentDir, lang.DirName, relPath)
			if _, err := os.Stat(langFile); os.IsNotExist(err) {
				missingIn = append(missingIn, lang.DirName)
			}
		}

		if len(missingIn) > 0 {
			result.File = relPath
			result.Languages = missingIn
			break
		}
	}

	return result
}

// CheckChanged computes detailed changes for all English files since last translation.
func (c *Checker) CheckChanged() ChangedResult {
	changedFiles := c.getCommittedChanges()

	result := ChangedResult{
		CheckpointTag:    c.config.CheckpointTag,
		CheckpointExists: c.checkpointExists(),
	}

	for _, file := range changedFiles {
		relPath := strings.TrimPrefix(file, c.sourcePath()+"/")
		cf := ChangedFile{
			Path: relPath,
		}

		// Get summary stats
		statCmd := exec.Command("git", "diff", "--stat", c.config.CheckpointTag+"..HEAD", "--", file)
		statOut, _ := statCmd.Output()
		lines := strings.Split(string(statOut), "\n")
		if len(lines) > 1 {
			// Parse "+X -Y" from stat output
			statLine := strings.TrimSpace(lines[len(lines)-2])
			// Simple parsing - could be improved
			if strings.Contains(statLine, "insertion") {
				// e.g. "1 file changed, 5 insertions(+), 2 deletions(-)"
				cf.LinesAdded = countInStat(statLine, "insertion")
				cf.LinesRemoved = countInStat(statLine, "deletion")
			}
		}

		// Get preview lines
		diffCmd := exec.Command("git", "diff", c.config.CheckpointTag+"..HEAD", "--", file)
		diffOut, _ := diffCmd.Output()
		for _, line := range strings.Split(string(diffOut), "\n") {
			if (strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++")) ||
				(strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---")) {
				cf.Preview = append(cf.Preview, line)
				if len(cf.Preview) >= 10 {
					break
				}
			}
		}

		result.Files = append(result.Files, cf)
	}

	return result
}

// countInStat extracts count from git stat output like "5 insertions(+)"
func countInStat(line, keyword string) int {
	parts := strings.Fields(line)
	for i, part := range parts {
		if strings.HasPrefix(part, keyword) && i > 0 {
			var count int
			_, _ = strings.NewReader(parts[i-1]).Read([]byte{byte(count)})
			// Simple atoi
			n := 0
			for _, c := range parts[i-1] {
				if c >= '0' && c <= '9' {
					n = n*10 + int(c-'0')
				}
			}
			return n
		}
	}
	return 0
}

// CheckValidate verifies translator config matches Hugo config.
func (c *Checker) CheckValidate() ValidateResult {
	var result ValidateResult

	// Check if this is a Hugo project
	if !IsHugoProject() {
		result.Warnings = append(result.Warnings, "No Hugo config found - using default configuration")
		return result
	}

	// Validate against Hugo config
	result.Mismatches = ValidateHugoConfig(c.config)
	return result
}

// CheckLangs computes configured languages and detects stray content directories.
func (c *Checker) CheckLangs() LangsResult {
	var result LangsResult

	// Build set of known directories
	knownDirs := make(map[string]bool)
	knownDirs[c.config.SourceDir] = true

	// Add source language
	result.Languages = append(result.Languages, LanguageInfo{
		Code:     c.config.SourceLang,
		Name:     "English",
		DirName:  c.config.SourceDir,
		Weight:   1,
		IsSource: true,
	})

	// Add target languages
	for i, lang := range c.config.TargetLangs {
		knownDirs[lang.DirName] = true
		result.Languages = append(result.Languages, LanguageInfo{
			Code:     lang.Code,
			Name:     lang.Name,
			DirName:  lang.DirName,
			Weight:   i + 2,
			IsSource: false,
		})
	}

	// Scan for stray directories
	entries, err := os.ReadDir(c.config.ContentDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && !knownDirs[entry.Name()] {
				result.StrayDirs = append(result.StrayDirs, entry.Name())
			}
		}
	}

	return result
}

// CheckMenu validates all menu files for broken links and sync issues.
func (c *Checker) CheckMenu() MenuCheckResult {
	var result MenuCheckResult

	// Parse English menu (source of truth)
	enMenuPath := GetMenuFilePath("en")
	enMenu, err := ParseMenuFile(enMenuPath)
	if err != nil {
		result.BrokenLinks = append(result.BrokenLinks, MenuIssue{
			Language: "en",
			Type:     "broken_link",
			Message:  "Cannot read English menu: " + err.Error(),
		})
		return result
	}

	// Check English menu links
	enLinkIssues := ValidateMenuLinks(enMenu, "en", c.config.ContentDir)
	for _, issue := range enLinkIssues {
		result.BrokenLinks = append(result.BrokenLinks, MenuIssue{
			Language: "en",
			Type:     "broken_link",
			Message:  issue,
		})
	}

	// Check each target language
	for _, lang := range c.config.TargetLangs {
		menuPath := GetMenuFilePath(lang.Code)
		targetMenu, err := ParseMenuFile(menuPath)
		if err != nil {
			result.BrokenLinks = append(result.BrokenLinks, MenuIssue{
				Language: lang.Code,
				Type:     "broken_link",
				Message:  "Cannot read menu file: " + err.Error(),
			})
			continue
		}

		// Check link validity
		linkIssues := ValidateMenuLinks(targetMenu, lang.Code, c.config.ContentDir)
		for _, issue := range linkIssues {
			result.BrokenLinks = append(result.BrokenLinks, MenuIssue{
				Language: lang.Code,
				Type:     "broken_link",
				Message:  issue,
			})
		}

		// Check structure sync with English
		structureDiffs := CompareMenuStructure(enMenu, targetMenu, lang.Code)
		for _, diff := range structureDiffs {
			result.SyncIssues = append(result.SyncIssues, MenuIssue{
				Language: lang.Code,
				Type:     "sync_issue",
				Message:  diff,
			})
		}
	}

	return result
}

// ============================================================================
// Private Helper Methods (unchanged from original)
// ============================================================================

func (c *Checker) getNewFiles() []string {
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard", "--", c.sourcePath()+"/")
	output, _ := cmd.Output()
	var files []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && strings.HasSuffix(line, ".md") {
			files = append(files, line)
		}
	}
	return files
}

func (c *Checker) getUncommittedChanges() []string {
	cmd := exec.Command("git", "diff", "--name-only", "--", c.sourcePath()+"/", "config/_default/menus.en.toml", "i18n/en.yaml")
	output, _ := cmd.Output()
	var files []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasSuffix(line, ".md") || strings.HasSuffix(line, ".toml") || strings.HasSuffix(line, ".yaml")) {
			files = append(files, line)
		}
	}
	return files
}

func (c *Checker) getCommittedChanges() []string {
	if !c.checkpointExists() {
		return nil
	}
	cmd := exec.Command("git", "diff", "--name-only", c.config.CheckpointTag+"..HEAD", "--", c.sourcePath()+"/", "config/_default/menus.en.toml", "i18n/en.yaml")
	output, _ := cmd.Output()
	var files []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasSuffix(line, ".md") || strings.HasSuffix(line, ".toml") || strings.HasSuffix(line, ".yaml")) {
			files = append(files, line)
		}
	}
	return files
}

func (c *Checker) checkpointExists() bool {
	cmd := exec.Command("git", "tag", "-l", c.config.CheckpointTag)
	output, _ := cmd.Output()
	return strings.TrimSpace(string(output)) != ""
}

func (c *Checker) getEnglishFiles() []string {
	var files []string
	filepath.Walk(c.sourcePath(), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files
}
