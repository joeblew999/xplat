// Package translator provides translation workflow management.
//
// This file defines result types for all translator commands.
// These structs carry computed data (no printing) and are passed to presenters for output.
package translator

// ============================================================================
// Query Results (read-only commands)
// ============================================================================

// StatusResult contains status check data
type StatusResult struct {
	NewFiles           []string // Files not yet tracked by git
	UncommittedChanges []string // Modified files not yet committed
	CommittedChanges   []string // Committed since last translation checkpoint
	CheckpointExists   bool     // Whether translation-done tag exists
	CheckpointTag      string   // The checkpoint tag name
}

// HasIssues returns true if there are any changes that need attention
func (r StatusResult) HasIssues() bool {
	return len(r.NewFiles) > 0 || len(r.UncommittedChanges) > 0 || len(r.CommittedChanges) > 0
}

// DiffResult contains diff output for a specific file
type DiffResult struct {
	File       string // The file path
	IsNew      bool   // Whether file is new (not in checkpoint)
	DiffOutput string // The git diff output
	Error      error  // Any error that occurred
}

// MissingResult contains missing translation data
type MissingResult struct {
	ByLanguage map[string][]string // lang code → missing files
	TotalCount int                 // Total missing files across all languages
}

// HasIssues returns true if any translations are missing
func (r MissingResult) HasIssues() bool {
	return r.TotalCount > 0
}

// StaleFile represents a potentially outdated translation
type StaleFile struct {
	SourcePath string  // English source file
	TargetPath string  // Translation file
	LangCode   string  // Language code
	SourceSize int64   // Size of English file
	TargetSize int64   // Size of translation
	Ratio      float64 // Target/Source ratio (< 0.5 is suspicious)
}

// StaleResult contains stale translation data
type StaleResult struct {
	Files []StaleFile
}

// HasIssues returns true if any stale translations found
func (r StaleResult) HasIssues() bool {
	return len(r.Files) > 0
}

// OrphansResult contains orphaned file data
type OrphansResult struct {
	ByLanguage map[string][]string // lang code → orphaned files
	TotalCount int                 // Total orphans across all languages
}

// HasIssues returns true if any orphans found
func (r OrphansResult) HasIssues() bool {
	return r.TotalCount > 0
}

// NextResult contains the next file to translate
type NextResult struct {
	File         string   // Next English file to translate
	Languages    []string // Languages that need this file
	TotalFiles   int      // Total English content files
	TranslatedBy map[string]int // Files translated per language
	MissingBy    map[string]int // Files missing per language
	AllDone      bool     // True if everything is translated
}

// ChangedFile represents a file that changed since checkpoint
type ChangedFile struct {
	Path        string   // File path
	LinesAdded  int      // Lines added
	LinesRemoved int     // Lines removed
	Preview     []string // First N lines of diff
}

// ChangedResult contains detailed change information
type ChangedResult struct {
	Files            []ChangedFile
	CheckpointTag    string
	CheckpointExists bool
}

// ValidateResult contains config validation data
type ValidateResult struct {
	Mismatches []string // Config mismatches found
	Warnings   []string // Non-fatal issues
}

// HasIssues returns true if validation failed
func (r ValidateResult) HasIssues() bool {
	return len(r.Mismatches) > 0
}

// LanguageInfo represents a configured language
type LanguageInfo struct {
	Code    string
	Name    string
	DirName string
	Weight  int
	IsSource bool
}

// LangsResult contains language configuration data
type LangsResult struct {
	Languages   []LanguageInfo // Configured languages
	StrayDirs   []string       // Content dirs not in config
}

// HasIssues returns true if stray directories found
func (r LangsResult) HasIssues() bool {
	return len(r.StrayDirs) > 0
}

// MenuIssue represents a menu validation problem
type MenuIssue struct {
	Language string // Which language menu has the issue
	Type     string // "broken_link" or "sync_issue"
	URL      string // The problematic URL
	Message  string // Description of the issue
}

// MenuCheckResult contains menu validation data
type MenuCheckResult struct {
	BrokenLinks []MenuIssue // URLs pointing to non-existent pages
	SyncIssues  []MenuIssue // Menu structure differences from English
}

// HasIssues returns true if any menu problems found
func (r MenuCheckResult) HasIssues() bool {
	return len(r.BrokenLinks) > 0 || len(r.SyncIssues) > 0
}

// ============================================================================
// Mutation Results (commands that modify state)
// ============================================================================

// CleanResult contains orphan cleanup data
type CleanResult struct {
	FilesToDelete map[string][]string // lang code → files to delete
	TotalCount    int                 // Total files to delete
	Deleted       bool                // Whether deletion was performed
	Error         error               // Any error during deletion
}

// DoneResult contains checkpoint update data
type DoneResult struct {
	OldTag  string // Previous checkpoint (if any)
	NewTag  string // New checkpoint tag
	Commit  string // Commit SHA that was tagged
	Error   error  // Any error during update
}

// MenuSyncResult contains menu sync data
type MenuSyncResult struct {
	FilesWritten []string // Menu files that were written
	Error        error    // Any error during sync
}

// LangAddResult contains language add data
type LangAddResult struct {
	Code        string // Language code added
	Name        string // Language display name
	DirName     string // Content directory name
	ConfigPath  string // Path to languages.toml
	ContentPath string // Path to content directory created
	MenuPath    string // Path to menu file created
	Error       error  // Any error during add
}

// LangRemoveResult contains language remove data
type LangRemoveResult struct {
	Code        string   // Language code removed
	FilesRemoved int     // Number of files deleted
	Cancelled   bool     // Whether user cancelled
	Error       error    // Any error during remove
}

// LangInitResult contains language init data
type LangInitResult struct {
	Code        string // Language code
	DirName     string // Content directory name
	Path        string // Full path to directory
	AlreadyExists bool // Whether directory already existed
	FileCount   int    // Number of existing files (if already exists)
	Error       error  // Any error during init
}
