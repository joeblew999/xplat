package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joeblew999/xplat/internal/taskfile"
	"github.com/spf13/cobra"
)

// LintCmd is the command for linting Taskfiles.
var LintCmd = &cobra.Command{
	Use:   "lint [files...]",
	Short: "Lint Taskfiles for convention violations",
	Long: `Lint Taskfiles for convention violations that require human review.

Checks performed:
  - archetype-vars:  Required vars for detected archetype
  - archetype-tasks: Required tasks for detected archetype
  - check-deps-status: check:deps should have status: section
  - doc-header: Documentation header comment

If no files are specified, lints all Taskfiles in taskfiles/ directory.

Examples:
  xplat lint                              # Lint all taskfiles
  xplat lint taskfiles/Taskfile.dummy.yml # Lint specific file
  xplat lint --json                       # JSON output for CI
  xplat lint --strict                     # Treat warnings as errors
  xplat lint --fix                        # Also run fmt fixes`,
	RunE: runLint,
}

var (
	lintJSON     bool
	lintStrict   bool
	lintFix      bool
	lintArchetype string
)

func init() {
	LintCmd.Flags().BoolVar(&lintJSON, "json", false, "Output as JSON")
	LintCmd.Flags().BoolVar(&lintStrict, "strict", false, "Treat warnings as errors")
	LintCmd.Flags().BoolVar(&lintFix, "fix", false, "Also run fmt fixes")
	LintCmd.Flags().StringVar(&lintArchetype, "archetype", "", "Override archetype detection (tool|external|builder|aggregation)")
}

// LintResult represents the result of linting a file.
type LintResult struct {
	File       string               `json:"file"`
	Archetype  string               `json:"archetype"`
	Violations []taskfile.Violation `json:"violations"`
}

// LintOutput represents the JSON output format.
type LintOutput struct {
	Results    []LintResult `json:"results"`
	TotalFiles int          `json:"total_files"`
	Errors     int          `json:"errors"`
	Warnings   int          `json:"warnings"`
}

func runLint(cmd *cobra.Command, args []string) error {
	// Find files to lint
	files, err := getTaskfilesToProcess(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No Taskfiles found")
		return nil
	}

	// If --fix is set, run fmt first
	if lintFix {
		if err := runFmtOnFiles(files); err != nil {
			return err
		}
	}

	// Collect all rules
	lintRules := taskfile.AllLintRules()
	fmtRules := taskfile.AllFmtRules() // Also check fmt rules as lint violations

	output := LintOutput{
		Results: make([]LintResult, 0),
	}

	for _, file := range files {
		tf, err := taskfile.Parse(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file, err)
			continue
		}

		output.TotalFiles++

		// Detect archetype
		info := taskfile.DetectArchetype(tf)

		// Collect violations
		var violations []taskfile.Violation

		// Apply lint rules
		for _, rule := range lintRules {
			violations = append(violations, rule.Check(tf)...)
		}

		// Also check fmt rules (but don't fix, just report)
		for _, rule := range fmtRules {
			violations = append(violations, rule.Check(tf)...)
		}

		// Count by severity
		for _, v := range violations {
			if v.Severity == taskfile.SeverityError {
				output.Errors++
			} else {
				output.Warnings++
			}
		}

		if len(violations) > 0 {
			output.Results = append(output.Results, LintResult{
				File:       file,
				Archetype:  string(info.Type),
				Violations: violations,
			})
		}
	}

	// Output results
	if lintJSON {
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
	} else {
		// Text output
		for _, result := range output.Results {
			for _, v := range result.Violations {
				fmt.Println(v.String())
			}
		}

		if output.Errors > 0 || output.Warnings > 0 {
			fmt.Printf("\nLint: %d error(s), %d warning(s) in %d file(s)\n",
				output.Errors, output.Warnings, output.TotalFiles)
		} else {
			fmt.Printf("Lint: %d file(s) OK\n", output.TotalFiles)
		}
	}

	// Determine exit code
	if output.Errors > 0 {
		return fmt.Errorf("lint failed with %d errors", output.Errors)
	}
	if lintStrict && output.Warnings > 0 {
		return fmt.Errorf("lint failed with %d warnings (strict mode)", output.Warnings)
	}

	return nil
}

// runFmtOnFiles runs fmt on the given files.
func runFmtOnFiles(files []string) error {
	rules := taskfile.AllFmtRules()

	for _, file := range files {
		tf, err := taskfile.Parse(file)
		if err != nil {
			continue // Skip unparseable files
		}

		// Apply fixes
		content := tf.RawContent
		for _, rule := range rules {
			tf2 := &taskfile.Taskfile{
				Path:       tf.Path,
				RawContent: content,
			}
			fixed, err := rule.Fix(tf2)
			if err != nil {
				continue
			}
			content = fixed
		}

		// Write if changed
		if string(content) != string(tf.RawContent) {
			if err := os.WriteFile(file, content, 0644); err != nil {
				return err
			}
			fmt.Printf("Fixed: %s\n", file)
		}
	}

	return nil
}
