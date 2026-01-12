package cmd

import (
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/processcompose"
	"github.com/spf13/cobra"
)

// ProcessFmtCmd is the command for formatting process-compose files.
var ProcessFmtCmd = &cobra.Command{
	Use:   "fmt [files...]",
	Short: "Format process-compose files",
	Long: `Format process-compose.yaml files with auto-fixes.

Fixes applied:
  - version:         Adds version field if missing
  - sort-processes:  Sorts processes alphabetically (coming soon)

If no files specified, auto-detects process-compose config in current directory.

Examples:
  xplat process fmt                           # Format auto-detected config
  xplat process fmt process-compose.yaml      # Format specific file
  xplat process fmt --check                   # Check without modifying`,
	RunE: runProcessFmt,
}

var (
	processFmtCheck bool
)

func init() {
	ProcessFmtCmd.Flags().BoolVar(&processFmtCheck, "check", false, "Check formatting without modifying files")
}

func runProcessFmt(cmd *cobra.Command, args []string) error {
	// Find files to format
	files, err := getProcessComposeFiles(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No process-compose files found")
		return nil
	}

	rules := processcompose.AllFmtRules()
	hasChanges := false

	for _, file := range files {
		pc, err := processcompose.Parse(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file, err)
			continue
		}

		// Apply fixes
		content := pc.RawContent
		for _, rule := range rules {
			// Check if rule has violations
			violations := rule.Check(pc)
			if len(violations) == 0 {
				continue
			}

			// Create a new ProcessCompose with current content
			pc2 := &processcompose.ProcessCompose{
				Path:       pc.Path,
				RawContent: content,
			}
			fixed, err := rule.Fix(pc2)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fixing %s with rule %s: %v\n", file, rule.Name(), err)
				continue
			}
			content = fixed
		}

		// Check if content changed
		if string(content) != string(pc.RawContent) {
			hasChanges = true
			if processFmtCheck {
				fmt.Printf("Would fix: %s\n", file)
			} else {
				if err := os.WriteFile(file, content, 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", file, err)
				}
				fmt.Printf("Fixed: %s\n", file)
			}
		}
	}

	if processFmtCheck && hasChanges {
		return fmt.Errorf("files need formatting")
	}

	if !hasChanges {
		fmt.Printf("All %d file(s) OK\n", len(files))
	}

	return nil
}
