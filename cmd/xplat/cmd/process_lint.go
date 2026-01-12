package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/processcompose"
	"github.com/spf13/cobra"
)

// ProcessLintCmd is the command for linting process-compose files.
var ProcessLintCmd = &cobra.Command{
	Use:   "lint [files...]",
	Short: "Lint process-compose files",
	Long: `Lint process-compose.yaml files for xplat conventions.

Checks performed:
  - task-command:      Commands should use 'task <subsystem>:run' pattern
  - task-health:       Readiness probes should use 'task <subsystem>:health'
  - shutdown-config:   Processes should have shutdown signal/timeout
  - restart-policy:    Processes should have restart policy
  - env-substitution:  Secrets should use ${VAR} substitution
  - env-file:          env_file should be configured if using ${VAR}
  - readiness-probe:   Processes should have readiness probes

If no files specified, auto-detects process-compose config in current directory.

Examples:
  xplat process lint                           # Lint auto-detected config
  xplat process lint process-compose.yaml      # Lint specific file
  xplat process lint --json                    # JSON output for CI
  xplat process lint --strict                  # Treat warnings as errors`,
	RunE: runProcessLint,
}

var (
	processLintJSON   bool
	processLintStrict bool
)

func init() {
	ProcessLintCmd.Flags().BoolVar(&processLintJSON, "json", false, "Output as JSON")
	ProcessLintCmd.Flags().BoolVar(&processLintStrict, "strict", false, "Treat warnings as errors")
}

// ProcessLintResult represents the result of linting a file.
type ProcessLintResult struct {
	File       string                     `json:"file"`
	Violations []processcompose.Violation `json:"violations"`
}

// ProcessLintOutput represents the JSON output format.
type ProcessLintOutput struct {
	Results    []ProcessLintResult `json:"results"`
	TotalFiles int                 `json:"total_files"`
	Errors     int                 `json:"errors"`
	Warnings   int                 `json:"warnings"`
}

func runProcessLint(cmd *cobra.Command, args []string) error {
	// Find files to lint
	files, err := getProcessComposeFiles(args)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No process-compose files found")
		return nil
	}

	// Collect all rules
	lintRules := processcompose.AllLintRules()
	fmtRules := processcompose.AllFmtRules() // Also check fmt rules as lint violations

	output := ProcessLintOutput{
		Results: make([]ProcessLintResult, 0),
	}

	for _, file := range files {
		pc, err := processcompose.Parse(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", file, err)
			continue
		}

		output.TotalFiles++

		// Collect violations
		var violations []processcompose.Violation

		// Apply lint rules
		for _, rule := range lintRules {
			violations = append(violations, rule.Check(pc)...)
		}

		// Also check fmt rules (but don't fix, just report)
		for _, rule := range fmtRules {
			violations = append(violations, rule.Check(pc)...)
		}

		// Count by severity
		for _, v := range violations {
			if v.Severity == processcompose.SeverityError {
				output.Errors++
			} else {
				output.Warnings++
			}
		}

		if len(violations) > 0 {
			output.Results = append(output.Results, ProcessLintResult{
				File:       file,
				Violations: violations,
			})
		}
	}

	// Output results
	if processLintJSON {
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
	if processLintStrict && output.Warnings > 0 {
		return fmt.Errorf("lint failed with %d warnings (strict mode)", output.Warnings)
	}

	return nil
}

// getProcessComposeFiles finds process-compose files to lint.
func getProcessComposeFiles(args []string) ([]string, error) {
	if len(args) > 0 {
		// Use specified files
		var files []string
		for _, arg := range args {
			// Check if it's a directory
			info, err := os.Stat(arg)
			if err != nil {
				return nil, err
			}
			if info.IsDir() {
				// Search for process-compose files in directory
				dirFiles, err := findProcessComposeInDir(arg)
				if err != nil {
					return nil, err
				}
				files = append(files, dirFiles...)
			} else {
				files = append(files, arg)
			}
		}
		return files, nil
	}

	// Auto-detect in current directory
	for _, name := range config.ProcessComposeSearchOrder() {
		if _, err := os.Stat(name); err == nil {
			return []string{name}, nil
		}
	}

	return nil, nil
}

// findProcessComposeInDir searches for process-compose files in a directory.
func findProcessComposeInDir(dir string) ([]string, error) {
	var files []string

	// Check for standard names
	for _, name := range config.ProcessComposeSearchOrder() {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		}
	}

	return files, nil
}
