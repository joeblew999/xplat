// Package translator provides translation workflow management.
//
// This file defines presenters for formatting command output.
// Presenters implement the Presenter interface and handle all output formatting,
// keeping business logic (in checker.go) pure and testable.
package translator

import (
	"fmt"
	"io"
	"os"
)

// Presenter defines the interface for formatting command output.
// Each method corresponds to a command and formats its result appropriately.
type Presenter interface {
	// Query results (read-only commands)
	Status(r StatusResult)
	Diff(r DiffResult)
	Missing(r MissingResult)
	Stale(r StaleResult)
	Orphans(r OrphansResult)
	Next(r NextResult)
	Changed(r ChangedResult)
	Validate(r ValidateResult)
	Langs(r LangsResult)
	MenuCheck(r MenuCheckResult)

	// Mutation results (commands that modify state)
	Clean(r CleanResult)
	Done(r DoneResult)
	MenuSync(r MenuSyncResult)
	LangAdd(r LangAddResult)
	LangRemove(r LangRemoveResult)
	LangInit(r LangInitResult)
}

// ============================================================================
// Terminal Presenter - Human-readable output for CLI
// ============================================================================

// TerminalPresenter formats output for interactive terminal use.
// Uses clear section headers and human-readable formatting.
type TerminalPresenter struct {
	w io.Writer
}

// NewTerminalPresenter creates a presenter that writes to stdout.
func NewTerminalPresenter() *TerminalPresenter {
	return &TerminalPresenter{w: os.Stdout}
}

// NewTerminalPresenterTo creates a presenter that writes to a custom writer.
func NewTerminalPresenterTo(w io.Writer) *TerminalPresenter {
	return &TerminalPresenter{w: w}
}

func (p *TerminalPresenter) header(title string) {
	fmt.Fprintln(p.w, "========================================")
	fmt.Fprintln(p.w, title)
	fmt.Fprintln(p.w, "========================================")
	fmt.Fprintln(p.w)
}

func (p *TerminalPresenter) footer() {
	fmt.Fprintln(p.w, "========================================")
}

func (p *TerminalPresenter) section(title string) {
	fmt.Fprintf(p.w, "=== %s ===\n", title)
}

// Status formats translation status for terminal.
func (p *TerminalPresenter) Status(r StatusResult) {
	p.header("Translation Status")

	p.section("New (untracked) files")
	if len(r.NewFiles) > 0 {
		for _, f := range r.NewFiles {
			fmt.Fprintln(p.w, f)
		}
	} else {
		fmt.Fprintln(p.w, "(none)")
	}
	fmt.Fprintln(p.w)

	p.section("Uncommitted changes (modified)")
	if len(r.UncommittedChanges) > 0 {
		for _, f := range r.UncommittedChanges {
			fmt.Fprintln(p.w, f)
		}
	} else {
		fmt.Fprintln(p.w, "(none)")
	}
	fmt.Fprintln(p.w)

	p.section("Committed since last translation")
	if len(r.CommittedChanges) > 0 {
		for _, f := range r.CommittedChanges {
			fmt.Fprintln(p.w, f)
		}
	} else {
		if r.CheckpointExists {
			fmt.Fprintln(p.w, "(none)")
		} else {
			fmt.Fprintln(p.w, "(No checkpoint tag yet - run 'translate done' to set baseline)")
		}
	}
	fmt.Fprintln(p.w)

	p.footer()
	fmt.Fprintln(p.w, "To translate, ask Claude Code:")
	fmt.Fprintln(p.w, "  'Translate the changed files to all languages'")
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "After translating: translate done")
	p.footer()
}

// Diff formats diff output for terminal.
func (p *TerminalPresenter) Diff(r DiffResult) {
	p.header(fmt.Sprintf("Diff for: %s", r.File))

	if r.Error != nil {
		fmt.Fprintf(p.w, "ERROR: %v\n", r.Error)
		return
	}

	if r.IsNew {
		fmt.Fprintln(p.w, "STATUS: NEW FILE (did not exist at last translation checkpoint)")
		fmt.Fprintln(p.w)
		fmt.Fprintln(p.w, "Full content:")
		fmt.Fprintln(p.w, "----------------------------------------")
		fmt.Fprint(p.w, r.DiffOutput)
		fmt.Fprintln(p.w, "----------------------------------------")
	} else if r.DiffOutput == "" {
		fmt.Fprintln(p.w, "STATUS: NO CHANGES since last translation")
	} else {
		fmt.Fprintln(p.w, "STATUS: MODIFIED since last translation")
		fmt.Fprintln(p.w)
		fmt.Fprintln(p.w, "Changes:")
		fmt.Fprintln(p.w, "----------------------------------------")
		fmt.Fprint(p.w, r.DiffOutput)
		fmt.Fprintln(p.w, "----------------------------------------")
	}

	fmt.Fprintln(p.w)
	p.footer()
}

// Missing formats missing translations for terminal.
func (p *TerminalPresenter) Missing(r MissingResult) {
	p.header("Missing Content Files by Language")

	// Need ordered language list - we'll pass it through result
	for langCode, files := range r.ByLanguage {
		if len(files) > 0 {
			fmt.Fprintf(p.w, "MISSING: %s: Missing %d files\n", langCode, len(files))
			for _, f := range files {
				fmt.Fprintf(p.w, "  - %s\n", f)
			}
			fmt.Fprintln(p.w)
		} else {
			fmt.Fprintf(p.w, "OK: %s: Complete\n", langCode)
		}
	}

	p.footer()
}

// Stale formats stale translations for terminal.
func (p *TerminalPresenter) Stale(r StaleResult) {
	p.header("Potentially Stale Translations")
	fmt.Fprintln(p.w, "(target file exists but is much smaller than English)")
	p.footer()
	fmt.Fprintln(p.w)

	if len(r.Files) == 0 {
		fmt.Fprintln(p.w, "OK: No stale translations found")
	} else {
		for _, f := range r.Files {
			fmt.Fprintf(p.w, "STALE: %s (English: %d bytes, %s: %d bytes)\n",
				f.TargetPath, f.SourceSize, f.LangCode, f.TargetSize)
		}
		fmt.Fprintln(p.w)
		fmt.Fprintf(p.w, "Found %d potentially stale files\n", len(r.Files))
		fmt.Fprintln(p.w, "Review and re-translate if needed")
	}
	p.footer()
}

// Orphans formats orphaned files for terminal.
func (p *TerminalPresenter) Orphans(r OrphansResult) {
	p.header("Orphaned Files (exist in target but not in English)")

	for langCode, files := range r.ByLanguage {
		if len(files) > 0 {
			fmt.Fprintf(p.w, "ORPHANS: %s: %d orphaned files (DELETE THESE)\n", langCode, len(files))
			for _, f := range files {
				fmt.Fprintf(p.w, "  - %s\n", f)
			}
			fmt.Fprintln(p.w)
		} else {
			fmt.Fprintf(p.w, "OK: %s: No orphans\n", langCode)
		}
	}

	p.footer()
	if r.TotalCount > 0 {
		fmt.Fprintln(p.w, "Run 'translate clean' to delete all orphaned files")
	}
	p.footer()
}

// Next formats next file to translate for terminal.
func (p *TerminalPresenter) Next(r NextResult) {
	p.header("")

	if r.AllDone {
		fmt.Fprintln(p.w, "All files translated!")
		p.footer()
		return
	}

	// Calculate totals
	totalPossible := r.TotalFiles * len(r.Languages)
	totalMissing := 0
	for _, count := range r.MissingBy {
		totalMissing += count
	}
	completed := totalPossible - totalMissing

	fmt.Fprintf(p.w, "Progress: %d/%d translations complete (%d remaining)\n", completed, totalPossible, totalMissing)
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "Next file to translate:")
	fmt.Fprintf(p.w, "  %s\n", r.File)
	fmt.Fprintln(p.w)

	// Show which languages need this file
	var missingIn []string
	for lang, count := range r.MissingBy {
		if count > 0 {
			missingIn = append(missingIn, lang)
		}
	}
	if len(r.Languages) > 0 {
		fmt.Fprintf(p.w, "Missing in: %v\n", r.Languages)
	}
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "To translate, ask Claude Code:")
	fmt.Fprintf(p.w, "  'Translate %s to all languages'\n", r.File)
	p.footer()
}

// Changed formats detailed changes for terminal.
func (p *TerminalPresenter) Changed(r ChangedResult) {
	p.header("Detailed Changes Since Last Translation")

	if len(r.Files) == 0 {
		fmt.Fprintln(p.w, "No English files changed since last translation.")
		p.footer()
		return
	}

	fmt.Fprintf(p.w, "Found %d changed file(s):\n", len(r.Files))
	fmt.Fprintln(p.w)

	for _, file := range r.Files {
		fmt.Fprintf(p.w, "--- %s ---\n", file.Path)
		if file.LinesAdded > 0 || file.LinesRemoved > 0 {
			fmt.Fprintf(p.w, "  +%d -%d lines\n", file.LinesAdded, file.LinesRemoved)
		}
		if len(file.Preview) > 0 {
			fmt.Fprintln(p.w, "  Preview:")
			for _, line := range file.Preview {
				fmt.Fprintf(p.w, "    %s\n", line)
			}
		}
		fmt.Fprintln(p.w)
	}

	p.footer()
	fmt.Fprintln(p.w, "To see full diff for a file:")
	fmt.Fprintln(p.w, "  translate diff <path>")
	p.footer()
}

// Validate formats validation results for terminal.
func (p *TerminalPresenter) Validate(r ValidateResult) {
	p.header("Validating Translator Configuration")

	if len(r.Mismatches) > 0 {
		p.footer()
		fmt.Fprintf(p.w, "WARNING: %d mismatch(es) found\n", len(r.Mismatches))
		p.footer()
		for _, m := range r.Mismatches {
			fmt.Fprintf(p.w, "  • %s\n", m)
		}
		fmt.Fprintln(p.w)
		fmt.Fprintln(p.w, "This shouldn't happen - languages are auto-loaded from Hugo config.")
		fmt.Fprintln(p.w, "Check if config/_default/languages.toml changed after binary was built.")
		return
	}

	if len(r.Warnings) > 0 {
		for _, w := range r.Warnings {
			fmt.Fprintf(p.w, "Note: %s\n", w)
		}
		fmt.Fprintln(p.w)
	}

	p.footer()
	fmt.Fprintln(p.w, "OK: Configuration valid")
	p.footer()
}

// Langs formats language configuration for terminal.
func (p *TerminalPresenter) Langs(r LangsResult) {
	p.header("Language Configuration")

	// Show source language first
	for _, lang := range r.Languages {
		if lang.IsSource {
			fmt.Fprintf(p.w, "SOURCE: %s → content/%s/\n", lang.Code, lang.DirName)
			break
		}
	}
	fmt.Fprintln(p.w)

	// Show target languages
	fmt.Fprintln(p.w, "TARGETS:")
	for _, lang := range r.Languages {
		if !lang.IsSource {
			fmt.Fprintf(p.w, "  %s (%s) → content/%s/\n", lang.Code, lang.Name, lang.DirName)
		}
	}
	fmt.Fprintln(p.w)

	// Show stray directories
	if len(r.StrayDirs) > 0 {
		fmt.Fprintln(p.w, "WARNING: Stray directories (not in config):")
		for _, dir := range r.StrayDirs {
			fmt.Fprintf(p.w, "  content/%s/\n", dir)
		}
		fmt.Fprintln(p.w)
		fmt.Fprintln(p.w, "These directories may be from a removed language.")
		fmt.Fprintln(p.w, "If they should be deleted, remove them manually:")
		for _, dir := range r.StrayDirs {
			fmt.Fprintf(p.w, "  rm -rf content/%s/\n", dir)
		}
		fmt.Fprintln(p.w)
		p.footer()
		fmt.Fprintln(p.w, "ACTION NEEDED: Stray directories found")
		p.footer()
		return
	}

	p.footer()
	fmt.Fprintln(p.w, "OK: All content directories are configured")
	p.footer()
}

// MenuCheck formats menu validation for terminal.
func (p *TerminalPresenter) MenuCheck(r MenuCheckResult) {
	p.header("Menu Validation")

	if len(r.BrokenLinks) > 0 {
		p.section("Broken Links")
		for _, issue := range r.BrokenLinks {
			fmt.Fprintf(p.w, "  %s: %s\n", issue.Language, issue.Message)
		}
		fmt.Fprintln(p.w)
	}

	if len(r.SyncIssues) > 0 {
		p.section("Structure Sync Issues")
		for _, issue := range r.SyncIssues {
			fmt.Fprintf(p.w, "  %s: %s\n", issue.Language, issue.Message)
		}
		fmt.Fprintln(p.w)
	}

	if !r.HasIssues() {
		fmt.Fprintln(p.w, "OK: All menus valid and in sync")
	} else {
		fmt.Fprintf(p.w, "Found %d issue(s)\n", len(r.BrokenLinks)+len(r.SyncIssues))
		fmt.Fprintln(p.w)
		fmt.Fprintln(p.w, "To fix sync issues: translate menu sync")
	}

	p.footer()
}

// Clean formats orphan cleanup for terminal.
func (p *TerminalPresenter) Clean(r CleanResult) {
	if r.TotalCount == 0 {
		fmt.Fprintln(p.w, "OK: No orphaned files to delete")
		return
	}

	p.header("Files to be deleted:")

	for langName, files := range r.FilesToDelete {
		fmt.Fprintf(p.w, "\n%s: %d files\n", langName, len(files))
		for _, f := range files {
			fmt.Fprintf(p.w, "  - %s\n", f)
		}
	}
	fmt.Fprintf(p.w, "\nTotal: %d files\n", r.TotalCount)
	p.footer()

	if r.Error != nil {
		fmt.Fprintf(p.w, "Error: %v\n", r.Error)
		return
	}

	if r.Deleted {
		fmt.Fprintf(p.w, "\nOK: Deleted %d orphaned files\n", r.TotalCount)
	}
}

// Done formats checkpoint update for terminal.
func (p *TerminalPresenter) Done(r DoneResult) {
	if r.Error != nil {
		fmt.Fprintf(p.w, "Error updating checkpoint: %v\n", r.Error)
		return
	}
	fmt.Fprintln(p.w, "OK: Translation checkpoint updated to current commit")
}

// MenuSync formats menu sync for terminal.
func (p *TerminalPresenter) MenuSync(r MenuSyncResult) {
	p.header("Menu Sync")

	if r.Error != nil {
		fmt.Fprintf(p.w, "Error: %v\n", r.Error)
		return
	}

	for _, path := range r.FilesWritten {
		fmt.Fprintf(p.w, "Generated: %s\n", path)
	}
	fmt.Fprintln(p.w)

	p.footer()
	fmt.Fprintln(p.w, "OK: Menu files regenerated from English")
	p.footer()
}

// LangAdd formats language add for terminal.
func (p *TerminalPresenter) LangAdd(r LangAddResult) {
	p.header(fmt.Sprintf("Adding language: %s (%s)", r.Code, r.Name))

	if r.Error != nil {
		fmt.Fprintf(p.w, "Error: %v\n", r.Error)
		return
	}

	fmt.Fprintf(p.w, "1. Added to %s\n", r.ConfigPath)
	fmt.Fprintf(p.w, "2. Created %s\n", r.ContentPath)
	if r.MenuPath != "" {
		fmt.Fprintf(p.w, "3. Generated %s\n", r.MenuPath)
	}
	fmt.Fprintln(p.w)

	p.footer()
	fmt.Fprintf(p.w, "OK: Language '%s' added\n", r.Code)
	p.footer()
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "Next steps:")
	fmt.Fprintln(p.w, "  1. Run 'task translate:missing' to see what needs translating")
	fmt.Fprintf(p.w, "  2. Translate content to %s\n", r.Name)
	fmt.Fprintln(p.w, "  3. Run 'task translate:done' when complete")
}

// LangRemove formats language remove for terminal.
func (p *TerminalPresenter) LangRemove(r LangRemoveResult) {
	p.header(fmt.Sprintf("Removing language: %s", r.Code))

	if r.Cancelled {
		fmt.Fprintln(p.w, "Cancelled")
		return
	}

	if r.Error != nil {
		fmt.Fprintf(p.w, "Error: %v\n", r.Error)
		return
	}

	fmt.Fprintf(p.w, "Deleted %d files\n", r.FilesRemoved)
	fmt.Fprintln(p.w)

	p.footer()
	fmt.Fprintf(p.w, "OK: Language '%s' removed\n", r.Code)
	p.footer()
}

// LangInit formats language init for terminal.
func (p *TerminalPresenter) LangInit(r LangInitResult) {
	p.header(fmt.Sprintf("Initializing content for language: %s", r.Code))

	if r.Error != nil {
		fmt.Fprintf(p.w, "Error: %v\n", r.Error)
		return
	}

	if r.AlreadyExists {
		fmt.Fprintf(p.w, "Directory %s already exists (%d .md files)\n", r.Path, r.FileCount)
		p.footer()
		return
	}

	fmt.Fprintf(p.w, "Created %s\n", r.Path)
	fmt.Fprintln(p.w)

	p.footer()
	fmt.Fprintf(p.w, "OK: Content directory initialized for '%s'\n", r.Code)
	p.footer()
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "Next steps:")
	fmt.Fprintln(p.w, "  Run 'task translate:missing' to see what needs translating")
}

// ============================================================================
// Markdown Presenter - GitHub Issue compatible output
// ============================================================================

// MarkdownPresenter formats output as GitHub-flavored markdown.
// Used for CI mode (-github-issue flag) to create issue-ready output.
type MarkdownPresenter struct {
	w io.Writer
}

// NewMarkdownPresenter creates a presenter that writes markdown to stdout.
func NewMarkdownPresenter() *MarkdownPresenter {
	return &MarkdownPresenter{w: os.Stdout}
}

// NewMarkdownPresenterTo creates a presenter that writes to a custom writer.
func NewMarkdownPresenterTo(w io.Writer) *MarkdownPresenter {
	return &MarkdownPresenter{w: w}
}

// Status formats translation status as markdown.
func (p *MarkdownPresenter) Status(r StatusResult) {
	if !r.HasIssues() {
		return // No output if no issues
	}

	fmt.Fprintln(p.w, "## Translation Status")
	fmt.Fprintln(p.w)

	if len(r.NewFiles) > 0 {
		fmt.Fprintln(p.w, "### New (untracked) files")
		for _, f := range r.NewFiles {
			fmt.Fprintf(p.w, "- `%s`\n", f)
		}
		fmt.Fprintln(p.w)
	}

	if len(r.UncommittedChanges) > 0 {
		fmt.Fprintln(p.w, "### Uncommitted changes")
		for _, f := range r.UncommittedChanges {
			fmt.Fprintf(p.w, "- `%s`\n", f)
		}
		fmt.Fprintln(p.w)
	}

	if len(r.CommittedChanges) > 0 {
		fmt.Fprintln(p.w, "### Committed since last translation")
		for _, f := range r.CommittedChanges {
			fmt.Fprintf(p.w, "- `%s`\n", f)
		}
		fmt.Fprintln(p.w)
	}
}

// Diff formats diff output as markdown (same as terminal for diffs).
func (p *MarkdownPresenter) Diff(r DiffResult) {
	fmt.Fprintf(p.w, "## Diff: %s\n\n", r.File)

	if r.Error != nil {
		fmt.Fprintf(p.w, "**Error:** %v\n", r.Error)
		return
	}

	if r.IsNew {
		fmt.Fprintln(p.w, "**Status:** New file\n")
		fmt.Fprintln(p.w, "```")
		fmt.Fprint(p.w, r.DiffOutput)
		fmt.Fprintln(p.w, "```")
	} else if r.DiffOutput == "" {
		fmt.Fprintln(p.w, "**Status:** No changes")
	} else {
		fmt.Fprintln(p.w, "**Status:** Modified\n")
		fmt.Fprintln(p.w, "```diff")
		fmt.Fprint(p.w, r.DiffOutput)
		fmt.Fprintln(p.w, "```")
	}
}

// Missing formats missing translations as markdown.
func (p *MarkdownPresenter) Missing(r MissingResult) {
	if r.TotalCount == 0 {
		return // No output if no issues
	}

	fmt.Fprintln(p.w, "## Missing Translations")
	fmt.Fprintln(p.w)

	for langCode, files := range r.ByLanguage {
		if len(files) > 0 {
			fmt.Fprintf(p.w, "### %s (%d files)\n", langCode, len(files))
			for _, f := range files {
				fmt.Fprintf(p.w, "- `%s`\n", f)
			}
			fmt.Fprintln(p.w)
		}
	}
}

// Stale formats stale translations as markdown.
func (p *MarkdownPresenter) Stale(r StaleResult) {
	if len(r.Files) == 0 {
		return // No output if no issues
	}

	fmt.Fprintln(p.w, "## Potentially Stale Translations")
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "These files are less than 50% the size of the English source:")
	fmt.Fprintln(p.w)
	for _, f := range r.Files {
		fmt.Fprintf(p.w, "- `%s` (EN: %d bytes, %s: %d bytes)\n",
			f.TargetPath, f.SourceSize, f.LangCode, f.TargetSize)
	}
}

// Orphans formats orphaned files as markdown.
func (p *MarkdownPresenter) Orphans(r OrphansResult) {
	if r.TotalCount == 0 {
		return // No output if no issues
	}

	fmt.Fprintln(p.w, "## Orphaned Translation Files")
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "These files exist in target languages but not in English (should be deleted):")
	fmt.Fprintln(p.w)

	for langCode, files := range r.ByLanguage {
		if len(files) > 0 {
			fmt.Fprintf(p.w, "### %s\n", langCode)
			for _, f := range files {
				fmt.Fprintf(p.w, "- `%s`\n", f)
			}
			fmt.Fprintln(p.w)
		}
	}
}

// Next formats next file info as markdown.
func (p *MarkdownPresenter) Next(r NextResult) {
	if r.AllDone {
		fmt.Fprintln(p.w, "## All Translations Complete ✓")
		return
	}

	fmt.Fprintln(p.w, "## Next Translation Needed")
	fmt.Fprintln(p.w)
	fmt.Fprintf(p.w, "**File:** `%s`\n\n", r.File)
	fmt.Fprintf(p.w, "**Missing in:** %v\n", r.Languages)
}

// Changed formats detailed changes as markdown.
func (p *MarkdownPresenter) Changed(r ChangedResult) {
	if len(r.Files) == 0 {
		return
	}

	fmt.Fprintln(p.w, "## Changed Files Since Last Translation")
	fmt.Fprintln(p.w)

	for _, file := range r.Files {
		fmt.Fprintf(p.w, "### `%s`\n", file.Path)
		if file.LinesAdded > 0 || file.LinesRemoved > 0 {
			fmt.Fprintf(p.w, "+%d -%d lines\n\n", file.LinesAdded, file.LinesRemoved)
		}
		if len(file.Preview) > 0 {
			fmt.Fprintln(p.w, "```diff")
			for _, line := range file.Preview {
				fmt.Fprintln(p.w, line)
			}
			fmt.Fprintln(p.w, "```")
		}
		fmt.Fprintln(p.w)
	}
}

// Validate formats validation results as markdown.
func (p *MarkdownPresenter) Validate(r ValidateResult) {
	if !r.HasIssues() {
		return
	}

	fmt.Fprintln(p.w, "## Configuration Validation Issues")
	fmt.Fprintln(p.w)

	for _, m := range r.Mismatches {
		fmt.Fprintf(p.w, "- %s\n", m)
	}
}

// Langs formats language configuration as markdown.
func (p *MarkdownPresenter) Langs(r LangsResult) {
	fmt.Fprintln(p.w, "## Language Configuration")
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "| Code | Name | Directory | Source |")
	fmt.Fprintln(p.w, "|------|------|-----------|--------|")

	for _, lang := range r.Languages {
		source := ""
		if lang.IsSource {
			source = "✓"
		}
		fmt.Fprintf(p.w, "| %s | %s | content/%s/ | %s |\n",
			lang.Code, lang.Name, lang.DirName, source)
	}

	if len(r.StrayDirs) > 0 {
		fmt.Fprintln(p.w)
		fmt.Fprintln(p.w, "### ⚠️ Stray Directories")
		fmt.Fprintln(p.w)
		for _, dir := range r.StrayDirs {
			fmt.Fprintf(p.w, "- `content/%s/`\n", dir)
		}
	}
}

// MenuCheck formats menu validation as markdown.
func (p *MarkdownPresenter) MenuCheck(r MenuCheckResult) {
	if !r.HasIssues() {
		return
	}

	fmt.Fprintln(p.w, "## Menu Validation Issues")
	fmt.Fprintln(p.w)

	if len(r.BrokenLinks) > 0 {
		fmt.Fprintln(p.w, "### Broken Links")
		for _, issue := range r.BrokenLinks {
			fmt.Fprintf(p.w, "- `%s`: %s\n", issue.Language, issue.Message)
		}
		fmt.Fprintln(p.w)
	}

	if len(r.SyncIssues) > 0 {
		fmt.Fprintln(p.w, "### Structure Sync Issues")
		for _, issue := range r.SyncIssues {
			fmt.Fprintf(p.w, "- `%s`: %s\n", issue.Language, issue.Message)
		}
		fmt.Fprintln(p.w)
	}

	fmt.Fprintln(p.w, "Run `task translate:menu:sync` to regenerate menu files from English.")
}

// Clean formats orphan cleanup as markdown.
func (p *MarkdownPresenter) Clean(r CleanResult) {
	if r.TotalCount == 0 {
		return
	}

	fmt.Fprintln(p.w, "## Orphan Cleanup")
	fmt.Fprintln(p.w)

	for langName, files := range r.FilesToDelete {
		fmt.Fprintf(p.w, "### %s (%d files)\n", langName, len(files))
		for _, f := range files {
			fmt.Fprintf(p.w, "- `%s`\n", f)
		}
		fmt.Fprintln(p.w)
	}

	if r.Error != nil {
		fmt.Fprintf(p.w, "**Error:** %v\n", r.Error)
	} else if r.Deleted {
		fmt.Fprintf(p.w, "**Deleted:** %d files\n", r.TotalCount)
	}
}

// Done formats checkpoint update as markdown.
func (p *MarkdownPresenter) Done(r DoneResult) {
	if r.Error != nil {
		fmt.Fprintf(p.w, "## Checkpoint Update Failed\n\n**Error:** %v\n", r.Error)
		return
	}
	fmt.Fprintf(p.w, "## Checkpoint Updated\n\nTag `%s` now points to `%s`\n", r.NewTag, r.Commit)
}

// MenuSync formats menu sync as markdown.
func (p *MarkdownPresenter) MenuSync(r MenuSyncResult) {
	if r.Error != nil {
		fmt.Fprintf(p.w, "## Menu Sync Failed\n\n**Error:** %v\n", r.Error)
		return
	}

	fmt.Fprintln(p.w, "## Menu Sync Complete")
	fmt.Fprintln(p.w)
	fmt.Fprintln(p.w, "Generated files:")
	for _, path := range r.FilesWritten {
		fmt.Fprintf(p.w, "- `%s`\n", path)
	}
}

// LangAdd formats language add as markdown.
func (p *MarkdownPresenter) LangAdd(r LangAddResult) {
	if r.Error != nil {
		fmt.Fprintf(p.w, "## Failed to Add Language\n\n**Error:** %v\n", r.Error)
		return
	}

	fmt.Fprintf(p.w, "## Added Language: %s (%s)\n\n", r.Code, r.Name)
	fmt.Fprintf(p.w, "- Config: `%s`\n", r.ConfigPath)
	fmt.Fprintf(p.w, "- Content: `%s`\n", r.ContentPath)
	if r.MenuPath != "" {
		fmt.Fprintf(p.w, "- Menu: `%s`\n", r.MenuPath)
	}
}

// LangRemove formats language remove as markdown.
func (p *MarkdownPresenter) LangRemove(r LangRemoveResult) {
	if r.Cancelled {
		fmt.Fprintln(p.w, "## Language Removal Cancelled")
		return
	}

	if r.Error != nil {
		fmt.Fprintf(p.w, "## Failed to Remove Language\n\n**Error:** %v\n", r.Error)
		return
	}

	fmt.Fprintf(p.w, "## Removed Language: %s\n\n", r.Code)
	fmt.Fprintf(p.w, "Deleted %d files.\n", r.FilesRemoved)
}

// LangInit formats language init as markdown.
func (p *MarkdownPresenter) LangInit(r LangInitResult) {
	if r.Error != nil {
		fmt.Fprintf(p.w, "## Failed to Initialize Language\n\n**Error:** %v\n", r.Error)
		return
	}

	if r.AlreadyExists {
		fmt.Fprintf(p.w, "## Language Directory Already Exists\n\n`%s` (%d files)\n", r.Path, r.FileCount)
		return
	}

	fmt.Fprintf(p.w, "## Initialized Language: %s\n\n", r.Code)
	fmt.Fprintf(p.w, "Created `%s`\n", r.Path)
}
