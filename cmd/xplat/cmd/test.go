package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joeblew999/xplat/internal/taskfile"
	"github.com/spf13/cobra"
)

var (
	testPhase  string
	testDryRun bool
	testInfo   bool
	testJSON   bool
)

// TestInfo contains metadata about a taskfile for CI decisions
type TestInfo struct {
	Path      string `json:"path"`
	Namespace string `json:"namespace"`
	Archetype string `json:"archetype"`
	Affinity  string `json:"affinity"` // "cross" or "native"
}

// TestCmd is the parent command for testing Taskfiles
var TestCmd = &cobra.Command{
	Use:   "test [taskfile|namespace]",
	Short: "Test a Taskfile based on its archetype",
	Long: `Test a Taskfile by running archetype-appropriate validation.

Each archetype has specific tests:

  TOOL:        check:deps, build, release:build, release:test, run
  EXTERNAL:    check:deps, check:validate (install and validate)
  BUILDER:     check:deps, build (with test vars)
  AGGREGATION: check:deps for all children
  BOOTSTRAP:   check:deps (self-bootstrap test)

Affinity determines cross-compile capability:
  cross:  Can cross-compile from single platform (CGO=0, pure code)
  native: Must build on native platform (CGO=1, native deps)

This command runs the same tests locally and in CI, eliminating
the need for per-tool workflow files.

Examples:
  # Test a specific taskfile
  xplat test taskfiles/Taskfile.dummy.yml

  # Test by namespace (finds taskfile automatically)
  xplat test dummy

  # Get info for CI workflow decisions (JSON)
  xplat test dummy --info --json

  # Run specific phase only
  xplat test dummy --phase=build

  # Dry run (show what would be tested)
  xplat test dummy --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runTest,
}

func init() {
	TestCmd.Flags().StringVar(&testPhase, "phase", "", "Run only a specific phase (deps, build, release, validate)")
	TestCmd.Flags().BoolVar(&testDryRun, "dry-run", false, "Show what would be tested without running")
	TestCmd.Flags().BoolVar(&testInfo, "info", false, "Output taskfile metadata only (for CI)")
	TestCmd.Flags().BoolVar(&testJSON, "json", false, "Output in JSON format (use with --info)")
}

func runTest(cmd *cobra.Command, args []string) error {
	target := args[0]

	// Resolve target to taskfile path
	taskfilePath, namespace, err := resolveTaskfileTarget(target)
	if err != nil {
		return err
	}

	// Parse taskfile
	tf, err := taskfile.Parse(taskfilePath)
	if err != nil {
		return fmt.Errorf("failed to parse taskfile: %w", err)
	}

	// Detect archetype and affinity
	arch := taskfile.DetectArchetype(tf)
	affinity := taskfile.DetectAffinity(tf, arch)

	// Build display archetype (includes affinity for tools)
	archetypeDisplay := strings.ToUpper(string(arch.Type))
	if arch.Type == taskfile.ArchetypeTool && affinity == taskfile.AffinityNative {
		archetypeDisplay = "TOOL-NATIVE"
	}

	// Info mode - just output metadata for CI
	if testInfo {
		info := TestInfo{
			Path:      taskfilePath,
			Namespace: namespace,
			Archetype: string(arch.Type),
			Affinity:  string(affinity),
		}
		if testJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(info)
		}
		fmt.Printf("path=%s\n", info.Path)
		fmt.Printf("namespace=%s\n", info.Namespace)
		fmt.Printf("archetype=%s\n", info.Archetype)
		fmt.Printf("affinity=%s\n", info.Affinity)
		return nil
	}

	fmt.Printf("Testing: %s\n", taskfilePath)
	fmt.Printf("Namespace: %s\n", namespace)
	fmt.Printf("Archetype: %s\n", archetypeDisplay)
	fmt.Printf("Affinity: %s\n", affinity)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()

	if testDryRun {
		return printTestPlan(arch, namespace, affinity)
	}

	// Run archetype-specific tests
	switch arch.Type {
	case taskfile.ArchetypeTool:
		return testTool(namespace, tf)
	case taskfile.ArchetypeExternal:
		return testExternal(namespace, tf)
	case taskfile.ArchetypeBuilder:
		return testBuilder(namespace, tf)
	case taskfile.ArchetypeAggregation:
		return testAggregation(namespace, tf)
	case taskfile.ArchetypeBootstrap:
		return testBootstrap(namespace, tf)
	default:
		return testUnknown(namespace, tf)
	}
}

// resolveTaskfileTarget converts a target (path or namespace) to taskfile path and namespace
func resolveTaskfileTarget(target string) (string, string, error) {
	// If it's a file path, use it directly
	if strings.HasSuffix(target, ".yml") || strings.HasSuffix(target, ".yaml") {
		if _, err := os.Stat(target); err != nil {
			return "", "", fmt.Errorf("taskfile not found: %s", target)
		}
		// Extract namespace from filename and path
		// For toolchain/Taskfile.golang.yml -> toolchain:golang
		// For tools/Taskfile.wrangler.yml -> wrangler (tools are top-level)
		// For Taskfile.dummy.yml -> dummy
		dir := filepath.Dir(target)
		base := filepath.Base(target)
		namespace := strings.TrimPrefix(base, "Taskfile.")
		namespace = strings.TrimSuffix(namespace, ".yml")
		namespace = strings.TrimSuffix(namespace, ".yaml")

		// Check if in a subdirectory that adds a prefix
		if strings.HasSuffix(dir, "/toolchain") || dir == "taskfiles/toolchain" {
			namespace = "toolchain:" + namespace
		}
		// Note: tools/* are typically included at root level (wrangler, pc, tui)
		// so they don't get a tools: prefix

		return target, namespace, nil
	}

	// It's a namespace - search for taskfile
	namespace := target

	// Handle colon-separated namespaces (e.g., toolchain:golang)
	if strings.Contains(namespace, ":") {
		parts := strings.SplitN(namespace, ":", 2)
		if parts[0] == "toolchain" {
			path := fmt.Sprintf("taskfiles/toolchain/Taskfile.%s.yml", parts[1])
			if _, err := os.Stat(path); err == nil {
				return path, namespace, nil
			}
		}
		if parts[0] == "tools" {
			path := fmt.Sprintf("taskfiles/tools/Taskfile.%s.yml", parts[1])
			if _, err := os.Stat(path); err == nil {
				// tools:* are typically at root level, not tools:* namespace
				return path, parts[1], nil
			}
		}
	}

	searchPaths := []string{
		fmt.Sprintf("taskfiles/Taskfile.%s.yml", namespace),
		fmt.Sprintf("taskfiles/tools/Taskfile.%s.yml", namespace),
		fmt.Sprintf("taskfiles/toolchain/Taskfile.%s.yml", namespace),
		fmt.Sprintf("Taskfile.%s.yml", namespace),
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			// Determine if prefix is needed
			if strings.Contains(path, "/toolchain/") {
				namespace = "toolchain:" + namespace
			}
			return path, namespace, nil
		}
	}

	return "", "", fmt.Errorf("could not find taskfile for namespace: %s\nSearched: %s", namespace, strings.Join(searchPaths, ", "))
}

func printTestPlan(arch taskfile.ArchetypeInfo, namespace string, affinity taskfile.Affinity) error {
	fmt.Println("Test Plan (dry-run):")
	fmt.Println()

	switch arch.Type {
	case taskfile.ArchetypeTool:
		fmt.Println("Phase 1: Dependencies")
		fmt.Printf("  - task %s:check:deps\n", namespace)
		fmt.Println()
		fmt.Println("Phase 2: Build")
		fmt.Printf("  - task %s:build\n", namespace)
		fmt.Println()
		fmt.Println("Phase 3: Release Build")
		fmt.Printf("  - task %s:release:build\n", namespace)
		fmt.Printf("  - task %s:release:test\n", namespace)
		fmt.Println()
		fmt.Println("Phase 4: Run")
		fmt.Printf("  - task %s:run\n", namespace)
		fmt.Println()
		if affinity == taskfile.AffinityNative {
			fmt.Println("CI Strategy: native - must build on each platform")
		} else {
			fmt.Println("CI Strategy: cross - can cross-compile from single platform")
		}

	case taskfile.ArchetypeExternal:
		fmt.Println("Phase 1: Dependencies")
		fmt.Printf("  - task %s:check:deps\n", namespace)
		fmt.Println()
		fmt.Println("Phase 2: Validate")
		fmt.Printf("  - task %s:check:validate (if exists)\n", namespace)
		fmt.Printf("  - task %s:version (if exists)\n", namespace)

	case taskfile.ArchetypeBuilder:
		fmt.Println("Phase 1: Dependencies")
		fmt.Printf("  - task %s:check:deps\n", namespace)
		fmt.Println()
		fmt.Println("Phase 2: Build (with dummy vars)")
		fmt.Printf("  - task %s:build (dry-run only - needs BIN, SOURCE vars)\n", namespace)

	case taskfile.ArchetypeAggregation:
		fmt.Println("Phase 1: Child Dependencies")
		fmt.Printf("  - task %s:check:deps\n", namespace)

	case taskfile.ArchetypeBootstrap:
		fmt.Println("Phase 1: Self-Bootstrap")
		fmt.Printf("  - task %s:check:deps\n", namespace)
		fmt.Println()
		fmt.Println("Phase 2: Validate")
		fmt.Printf("  - task %s:check:validate (if exists)\n", namespace)

	default:
		fmt.Println("Phase 1: Dependencies (if exists)")
		fmt.Printf("  - task %s:check:deps\n", namespace)
		fmt.Println()
		fmt.Println("Phase 2: Validate (if exists)")
		fmt.Printf("  - task %s:check:validate\n", namespace)
	}

	fmt.Println()
	return nil
}

func execTask(namespace, task string) error {
	fullTask := fmt.Sprintf("%s:%s", namespace, task)
	fmt.Printf(">>> Running: task %s\n", fullTask)

	// Use xplat's embedded Task via os.Args[0] to ensure it works in CI
	// where standalone Task may not be installed
	xplatBin, err := os.Executable()
	if err != nil {
		xplatBin = os.Args[0]
	}

	cmd := exec.Command(xplatBin, "task", fullTask)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("task %s failed: %w", fullTask, err)
	}

	fmt.Println()
	return nil
}

func execTaskIfExists(namespace, task string) error {
	fullTask := fmt.Sprintf("%s:%s", namespace, task)

	// Use xplat's embedded Task to check if task exists
	xplatBin, err := os.Executable()
	if err != nil {
		xplatBin = os.Args[0]
	}

	listCmd := exec.Command(xplatBin, "task", "--list", "--json")
	output, err := listCmd.Output()
	if err != nil {
		// If task list fails, try running anyway
		return execTask(namespace, task)
	}

	if !strings.Contains(string(output), fullTask) {
		fmt.Printf(">>> Skipping: task %s (not found)\n\n", fullTask)
		return nil
	}

	return execTask(namespace, task)
}

func testTool(namespace string, tf *taskfile.Taskfile) error {
	fmt.Println("=== Phase 1: Dependencies ===")
	if testPhase == "" || testPhase == "deps" {
		if err := execTask(namespace, "check:deps"); err != nil {
			return err
		}
	}

	fmt.Println("=== Phase 2: Build ===")
	if testPhase == "" || testPhase == "build" {
		if err := execTask(namespace, "build"); err != nil {
			return err
		}
	}

	fmt.Println("=== Phase 3: Release Build ===")
	if testPhase == "" || testPhase == "release" {
		if err := execTask(namespace, "release:build"); err != nil {
			return err
		}
		if err := execTask(namespace, "release:test"); err != nil {
			return err
		}
	}

	fmt.Println("=== Phase 4: Run ===")
	if testPhase == "" || testPhase == "validate" {
		if err := execTask(namespace, "run"); err != nil {
			return err
		}
	}

	fmt.Println("=== PASS: Tool tests completed ===")
	return nil
}

func testExternal(namespace string, tf *taskfile.Taskfile) error {
	fmt.Println("=== Phase 1: Dependencies ===")
	if testPhase == "" || testPhase == "deps" {
		if err := execTask(namespace, "check:deps"); err != nil {
			return err
		}
	}

	fmt.Println("=== Phase 2: Validate ===")
	if testPhase == "" || testPhase == "validate" {
		// Try check:validate first, fall back to version
		if err := execTaskIfExists(namespace, "check:validate"); err != nil {
			return err
		}
		if err := execTaskIfExists(namespace, "version"); err != nil {
			return err
		}
	}

	fmt.Println("=== PASS: External tests completed ===")
	return nil
}

func testBuilder(namespace string, tf *taskfile.Taskfile) error {
	fmt.Println("=== Phase 1: Dependencies ===")
	if testPhase == "" || testPhase == "deps" {
		if err := execTask(namespace, "check:deps"); err != nil {
			return err
		}
	}

	// Note: Builder's build task requires caller to provide vars
	// We can't test it generically without a test target
	fmt.Println("=== Phase 2: Build (skipped - requires caller vars) ===")
	fmt.Println("Builder archetypes provide build: task for Tools to call.")
	fmt.Println("Test via: task dummy:release:build (which calls :toolchain:golang:build)")
	fmt.Println()

	fmt.Println("=== PASS: Builder tests completed ===")
	return nil
}

func testAggregation(namespace string, tf *taskfile.Taskfile) error {
	fmt.Println("=== Phase 1: Child Dependencies ===")
	if testPhase == "" || testPhase == "deps" {
		if err := execTaskIfExists(namespace, "check:deps"); err != nil {
			return err
		}
	}

	fmt.Println("=== PASS: Aggregation tests completed ===")
	return nil
}

func testBootstrap(namespace string, tf *taskfile.Taskfile) error {
	fmt.Println("=== Phase 1: Self-Bootstrap ===")
	if testPhase == "" || testPhase == "deps" {
		if err := execTask(namespace, "check:deps"); err != nil {
			return err
		}
	}

	fmt.Println("=== Phase 2: Validate ===")
	if testPhase == "" || testPhase == "validate" {
		if err := execTaskIfExists(namespace, "check:validate"); err != nil {
			return err
		}
	}

	fmt.Println("=== PASS: Bootstrap tests completed ===")
	return nil
}

func testUnknown(namespace string, tf *taskfile.Taskfile) error {
	fmt.Println("=== Phase 1: Dependencies ===")
	if testPhase == "" || testPhase == "deps" {
		if err := execTaskIfExists(namespace, "check:deps"); err != nil {
			return err
		}
	}

	fmt.Println("=== Phase 2: Validate ===")
	if testPhase == "" || testPhase == "validate" {
		if err := execTaskIfExists(namespace, "check:validate"); err != nil {
			return err
		}
	}

	fmt.Println("=== PASS: Unknown archetype tests completed ===")
	return nil
}
