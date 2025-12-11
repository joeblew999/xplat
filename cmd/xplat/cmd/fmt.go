package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/xplat/internal/taskfile"
	"github.com/spf13/cobra"
)

// FmtCmd is the command for formatting Taskfiles.
var FmtCmd = &cobra.Command{
	Use:   "fmt [files...]",
	Short: "Format Taskfiles",
	Long: `Format Taskfiles by auto-fixing common issues.

Fixes applied:
  - Add {{exeExt}} to _BIN variables
  - Replace bare 'xplat' with {{.XPLAT_BIN}}
  - Quote echo statements containing colons

If no files are specified, formats all Taskfiles in taskfiles/ directory.

Examples:
  xplat fmt                              # Format all taskfiles
  xplat fmt taskfiles/Taskfile.dummy.yml # Format specific file
  xplat fmt --check                      # Check only, exit 1 if changes needed
  xplat fmt --diff                       # Show what would change`,
	RunE: runFmt,
}

var (
	fmtCheck bool
	fmtDiff  bool
)

func init() {
	FmtCmd.Flags().BoolVar(&fmtCheck, "check", false, "Check only, exit 1 if changes needed (for CI)")
	FmtCmd.Flags().BoolVar(&fmtDiff, "diff", false, "Show what would change")
}

func runFmt(cmd *cobra.Command, args []string) error {
	// Find files to format
	files, err := getTaskfilesToProcess(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No Taskfiles found")
		return nil
	}

	rules := taskfile.AllFmtRules()
	hasChanges := false
	totalFixed := 0

	for _, file := range files {
		tf, err := taskfile.Parse(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file, err)
			continue
		}

		// Check for violations
		var violations []taskfile.Violation
		for _, rule := range rules {
			violations = append(violations, rule.Check(tf)...)
		}

		if len(violations) == 0 {
			continue // File is clean
		}

		hasChanges = true

		if fmtCheck {
			// Just report violations
			for _, v := range violations {
				fmt.Println(v.String())
			}
			totalFixed += len(violations)
			continue
		}

		// Apply fixes
		content := tf.RawContent
		for _, rule := range rules {
			// Re-parse after each fix to get updated content
			tf2 := &taskfile.Taskfile{
				Path:       tf.Path,
				RawContent: content,
				Lines:      strings.Split(string(content), "\n"),
			}
			fixed, err := rule.Fix(tf2)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error applying %s to %s: %v\n", rule.Name(), file, err)
				continue
			}
			content = fixed
		}

		// Check if content changed
		if bytes.Equal(content, tf.RawContent) {
			continue
		}

		if fmtDiff {
			// Show diff
			fmt.Printf("--- %s\n+++ %s (formatted)\n", file, file)
			showSimpleDiff(string(tf.RawContent), string(content))
			totalFixed += len(violations)
			continue
		}

		// Write changes
		if err := os.WriteFile(file, content, 0644); err != nil {
			return fmt.Errorf("error writing %s: %w", file, err)
		}
		fmt.Printf("Formatted: %s (%d fixes)\n", file, len(violations))
		totalFixed += len(violations)
	}

	if fmtCheck && hasChanges {
		return fmt.Errorf("%d formatting issues found", totalFixed)
	}

	if !fmtCheck && !fmtDiff && totalFixed > 0 {
		fmt.Printf("\nFormatted %d file(s) with %d total fixes\n", len(files), totalFixed)
	}

	return nil
}

// getTaskfilesToProcess returns the list of Taskfiles to process.
func getTaskfilesToProcess(args []string) ([]string, error) {
	if len(args) > 0 {
		// Use provided files
		var files []string
		for _, arg := range args {
			if _, err := os.Stat(arg); err != nil {
				return nil, fmt.Errorf("file not found: %s", arg)
			}
			files = append(files, arg)
		}
		return files, nil
	}

	// Find all Taskfiles in taskfiles/ directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	taskfilesDir := filepath.Join(cwd, "taskfiles")
	if _, err := os.Stat(taskfilesDir); os.IsNotExist(err) {
		// Try current directory
		taskfilesDir = cwd
	}

	return taskfile.FindTaskfiles(taskfilesDir)
}

// showSimpleDiff shows a simple line-by-line diff.
func showSimpleDiff(old, new string) {
	oldLines := splitLines(old)
	newLines := splitLines(new)

	// Very simple diff - just show changed lines
	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine != newLine {
			if oldLine != "" {
				fmt.Printf("-%s\n", oldLine)
			}
			if newLine != "" {
				fmt.Printf("+%s\n", newLine)
			}
		}
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
