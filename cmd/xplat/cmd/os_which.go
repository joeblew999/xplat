// which.go - Find binaries by REFLECTING off Taskfiles
//
// DESIGN INTENT:
// 1. Users can add ANY Taskfile at runtime that conforms to our Archetypes
// 2. xplat os which REFLECTS off Taskfiles to discover install locations from vars:
//    - BUN_GLOBAL_BIN (e.g., ~/.bun/bin)
//    - <TOOL>_INSTALL_DIR (e.g., WRANGLER_INSTALL_DIR)
//    - BIN_INSTALL_DIR (e.g., ~/.local/bin)
// 3. Falls back to convention locations if no Taskfile found
// 4. Falls back to PATH as last resort
//
// OUTPUT PATTERN (follows xplat convention from test.go, lint.go):
// - Structs with `json` tags = single source of truth for both formats
// - Human output: key=value format (scannable, parseable)
// - JSON output: --json flag for machine consumption
//
// COMMANDS:
//   xplat os which <tool>           -> just the path (scripts use this)
//   xplat os which <tool> --all     -> all locations (debugging conflicts)
//   xplat os which doctor <tool>    -> full diagnostics (versions, conflicts)
//   Add --json to any for machine-parseable output
//
// WHY THIS MATTERS:
// - Developers may have same binary via brew AND Taskfile
// - Version conflicts cause subtle bugs
// - This tool helps debug "which binary am I actually running?"

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joeblew999/xplat/internal/taskfile"
	"github.com/spf13/cobra"
)

var (
	whichAll  bool
	whichJSON bool
)

// WhichResult represents a single binary location (JSON-serializable)
type WhichResult struct {
	Tool    string `json:"tool"`
	Path    string `json:"path"`
	Source  string `json:"source"` // taskfile, convention, path, brew
	Version string `json:"version,omitempty"`
	Active  bool   `json:"active"`
}

// WhichDoctorResult for full diagnostics (JSON-serializable)
type WhichDoctorResult struct {
	Tool            string        `json:"tool"`
	Found           bool          `json:"found"`
	ActivePath      string        `json:"active_path,omitempty"`
	ActiveVersion   string        `json:"active_version,omitempty"`
	ExpectedVersion string        `json:"expected_version,omitempty"` // From xplat.env
	VersionMatch    *bool         `json:"version_match,omitempty"`    // nil if can't compare
	Taskfile        string        `json:"taskfile,omitempty"`
	Archetype       string        `json:"archetype,omitempty"`
	ExpectedPath    string        `json:"expected_path,omitempty"`
	Installations   []WhichResult `json:"installations"`
	Status          string        `json:"status"` // ok, warn, error
	Issues          []string      `json:"issues,omitempty"`
	Warnings        []string      `json:"warnings,omitempty"`
}

// WhichCmd finds a binary in managed locations or PATH
var WhichCmd = &cobra.Command{
	Use:   "which <binary>",
	Short: "Find binary in managed locations or PATH",
	Long: `Find the path to an executable.

Checks xplat-managed locations first by reflecting off Taskfiles,
then falls back to PATH. This allows users to find tools without
modifying their shell configuration.

Use --all to show ALL locations where the binary exists.
Use --json for machine-parseable output.
Use 'xplat os which doctor <tool>' for detailed diagnostics.

Examples:
  xplat os which wrangler           # just the path
  xplat os which wrangler --json    # JSON output
  xplat os which wrangler --all     # all locations
  xplat os which doctor wrangler    # full diagnostics`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		if whichAll {
			runWhichAll(name)
			return
		}

		// Find the tool
		path := findManagedTool(name)
		source := "taskfile"
		if path == "" {
			var err error
			path, err = exec.LookPath(name)
			if err != nil {
				if whichJSON {
					result := WhichResult{Tool: name, Path: "", Source: "", Active: false}
					outputJSON(result)
				}
				os.Exit(1)
			}
			source = "path"
		}

		if whichJSON {
			version := getToolVersion(path, name)
			result := WhichResult{
				Tool:    name,
				Path:    path,
				Source:  source,
				Version: version,
				Active:  true,
			}
			outputJSON(result)
		} else {
			fmt.Println(path)
		}
	},
}

// WhichDoctorCmd provides detailed diagnostics for a tool
var WhichDoctorCmd = &cobra.Command{
	Use:   "doctor <binary>",
	Short: "Diagnose tool installation and version conflicts",
	Long: `Detailed diagnostics for a tool including version checks
and conflict detection. Use --json for machine-parseable output.

Examples:
  xplat os which doctor wrangler
  xplat os which doctor wrangler --json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runWhichDoctor(args[0])
	},
}

func init() {
	WhichCmd.Flags().BoolVarP(&whichAll, "all", "a", false, "Show all locations")
	WhichCmd.Flags().BoolVar(&whichJSON, "json", false, "Output in JSON format")
	WhichCmd.AddCommand(WhichDoctorCmd)
	WhichDoctorCmd.Flags().BoolVar(&whichJSON, "json", false, "Output in JSON format")
}

// outputJSON outputs data as JSON
func outputJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// runWhichAll shows all locations where the binary exists
func runWhichAll(name string) {
	home, _ := os.UserHomeDir()
	result := WhichDoctorResult{
		Tool:          name,
		Installations: []WhichResult{},
	}

	// Find active path
	activePath := findManagedTool(name)
	if activePath == "" {
		activePath, _ = exec.LookPath(name)
	}
	if activePath != "" {
		result.Found = true
		result.ActivePath = activePath
		result.ActiveVersion = getToolVersion(activePath, name)
	}

	// Check Taskfile location
	tf := findTaskfileForTool(name)
	if tf != nil {
		result.Taskfile = tf.Path
		if path := getInstallPathFromTaskfile(tf, name, home); path != "" {
			if fileExists(path) {
				version := getToolVersion(path, name)
				result.Installations = append(result.Installations, WhichResult{
					Tool:    name,
					Path:    path,
					Source:  "taskfile",
					Version: version,
					Active:  path == activePath,
				})
			}
		}
	}

	// Check convention locations
	for _, loc := range getConventionLocations(name, home) {
		if fileExists(loc) && !containsResultPath(result.Installations, loc) {
			version := getToolVersion(loc, name)
			result.Installations = append(result.Installations, WhichResult{
				Tool:    name,
				Path:    loc,
				Source:  "convention",
				Version: version,
				Active:  loc == activePath,
			})
		}
	}

	// Check PATH locations
	for _, inst := range findAllInPathWithVersions(name) {
		if !containsResultPath(result.Installations, inst.Path) {
			result.Installations = append(result.Installations, WhichResult{
				Tool:    name,
				Path:    inst.Path,
				Source:  inst.Source,
				Version: inst.Version,
				Active:  inst.Path == activePath,
			})
		}
	}

	// Set status
	if len(result.Installations) > 1 {
		result.Status = "warn"
		result.Warnings = append(result.Warnings, fmt.Sprintf("Multiple installations found (%d)", len(result.Installations)))
	} else if len(result.Installations) == 0 {
		result.Status = "error"
		result.Issues = append(result.Issues, "Not found")
	} else {
		result.Status = "ok"
	}

	if whichJSON {
		outputJSON(result)
	} else {
		// Human-readable key=value format
		fmt.Printf("tool=%s\n", name)
		if result.ActivePath != "" {
			fmt.Printf("active=%s\n", result.ActivePath)
		}
		if result.ActiveVersion != "" {
			fmt.Printf("version=%s\n", result.ActiveVersion)
		}
		if result.Taskfile != "" {
			fmt.Printf("taskfile=%s\n", result.Taskfile)
		}
		fmt.Printf("installations=%d\n", len(result.Installations))
		for _, inst := range result.Installations {
			activeMarker := ""
			if inst.Active {
				activeMarker = " active"
			}
			fmt.Printf("  %s [%s]%s\n", inst.Path, inst.Source, activeMarker)
		}
		fmt.Printf("status=%s\n", result.Status)
		for _, w := range result.Warnings {
			fmt.Printf("warning=%s\n", w)
		}
	}

	if !result.Found {
		os.Exit(1)
	}
}

// runWhichDoctor provides comprehensive diagnostics
func runWhichDoctor(name string) {
	home, _ := os.UserHomeDir()
	result := WhichDoctorResult{
		Tool:          name,
		Installations: []WhichResult{},
		Issues:        []string{},
		Warnings:      []string{},
	}

	// Find active path
	activePath := findManagedTool(name)
	if activePath == "" {
		activePath, _ = exec.LookPath(name)
	}
	if activePath != "" {
		result.Found = true
		result.ActivePath = activePath
		result.ActiveVersion = getToolVersion(activePath, name)
	}

	// Get expected version from xplat.env (source of truth)
	expectedVersion := getExpectedVersionFromEnv(name)
	if expectedVersion != "" {
		result.ExpectedVersion = expectedVersion
	}

	// Check Taskfile configuration
	tf := findTaskfileForTool(name)
	var expectedPath string

	if tf != nil {
		result.Taskfile = tf.Path
		arch := taskfile.DetectArchetype(tf)
		result.Archetype = strings.ToUpper(string(arch.Type))
		expectedPath = getInstallPathFromTaskfile(tf, name, home)
		result.ExpectedPath = expectedPath

		if fileExists(expectedPath) {
			version := getToolVersion(expectedPath, name)
			result.Installations = append(result.Installations, WhichResult{
				Tool:    name,
				Path:    expectedPath,
				Source:  "taskfile",
				Version: version,
				Active:  expectedPath == activePath,
			})
		}
	}

	// Check convention locations
	for _, loc := range getConventionLocations(name, home) {
		if fileExists(loc) && !containsResultPath(result.Installations, loc) {
			version := getToolVersion(loc, name)
			result.Installations = append(result.Installations, WhichResult{
				Tool:    name,
				Path:    loc,
				Source:  "convention",
				Version: version,
				Active:  loc == activePath,
			})
		}
	}

	// Check PATH locations
	for _, inst := range findAllInPathWithVersions(name) {
		if !containsResultPath(result.Installations, inst.Path) {
			result.Installations = append(result.Installations, WhichResult{
				Tool:    name,
				Path:    inst.Path,
				Source:  inst.Source,
				Version: inst.Version,
				Active:  inst.Path == activePath,
			})
		}
	}

	// Version comparison
	if expectedVersion != "" && result.ActiveVersion != "" {
		match := versionMatches(result.ActiveVersion, expectedVersion)
		result.VersionMatch = &match
		if !match {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Version mismatch: expected %s, installed %s", expectedVersion, result.ActiveVersion))
		}
	}

	// Diagnose issues
	if len(result.Installations) > 1 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Multiple installations found (%d locations)", len(result.Installations)))
	}

	if tf != nil && !fileExists(expectedPath) {
		result.Issues = append(result.Issues, fmt.Sprintf("Not installed at expected location: %s", expectedPath))
	}

	// Set status
	if len(result.Issues) > 0 {
		result.Status = "error"
	} else if len(result.Warnings) > 0 {
		result.Status = "warn"
	} else if result.Found {
		result.Status = "ok"
	} else {
		result.Status = "error"
		result.Issues = append(result.Issues, "Not found")
	}

	if whichJSON {
		outputJSON(result)
	} else {
		// Human-readable key=value format
		fmt.Printf("tool=%s\n", name)
		if result.ExpectedVersion != "" {
			fmt.Printf("expected=%s\n", result.ExpectedVersion)
		}
		if result.ActiveVersion != "" {
			fmt.Printf("installed=%s\n", result.ActiveVersion)
		}
		if result.VersionMatch != nil {
			fmt.Printf("match=%t\n", *result.VersionMatch)
		}
		if result.Taskfile != "" {
			fmt.Printf("taskfile=%s\n", result.Taskfile)
		} else {
			fmt.Printf("taskfile=(none)\n")
		}
		if result.Archetype != "" {
			fmt.Printf("archetype=%s\n", result.Archetype)
		}
		if result.ActivePath != "" {
			fmt.Printf("active=%s\n", result.ActivePath)
		}
		if len(result.Installations) > 1 {
			fmt.Printf("installations=%d\n", len(result.Installations))
			for _, inst := range result.Installations {
				activeMarker := ""
				if inst.Active {
					activeMarker = " active"
				}
				fmt.Printf("  %s [%s]%s\n", inst.Path, inst.Source, activeMarker)
			}
		}
		fmt.Printf("status=%s\n", result.Status)
		for _, issue := range result.Issues {
			fmt.Printf("issue=%s\n", issue)
		}
		for _, w := range result.Warnings {
			fmt.Printf("warning=%s\n", w)
		}
	}

	if result.Status == "error" {
		os.Exit(1)
	}
}

// containsResultPath checks if path is already in installations
func containsResultPath(installations []WhichResult, path string) bool {
	for _, inst := range installations {
		if inst.Path == path {
			return true
		}
	}
	return false
}

// --- Helper functions ---

func getToolVersion(path string, name string) string {
	versionFlags := []string{"--version", "-v", "version", "-V"}
	for _, flag := range versionFlags {
		cmd := exec.Command(path, flag)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Env = os.Environ()
		if err := cmd.Run(); err == nil {
			output := stdout.String()
			if output == "" {
				output = stderr.String()
			}
			if version := extractVersion(output, name); version != "" {
				return version
			}
		}
	}
	return ""
}

func extractVersion(output string, name string) string {
	output = strings.TrimSpace(output)
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return ""
	}
	line := strings.TrimSpace(lines[0])
	line = strings.TrimPrefix(line, name+" ")
	line = strings.TrimPrefix(line, name+"/")
	line = strings.TrimPrefix(line, "version ")
	line = strings.TrimPrefix(line, "Version ")
	line = strings.TrimPrefix(line, "v")
	line = strings.TrimPrefix(line, "V")
	if strings.HasPrefix(line, "go") {
		parts := strings.Fields(line)
		for _, p := range parts {
			if strings.HasPrefix(p, "go") && len(p) > 2 {
				return strings.TrimPrefix(p, "go")
			}
		}
	}
	parts := strings.Fields(line)
	if len(parts) > 0 {
		version := parts[0]
		version = strings.TrimRight(version, ":")
		return version
	}
	return ""
}

func versionMatches(installed, expected string) bool {
	installed = strings.TrimPrefix(installed, "v")
	expected = strings.TrimPrefix(expected, "v")
	if installed == expected {
		return true
	}
	if strings.HasPrefix(installed, expected) {
		return true
	}
	return false
}

// Installation represents a found binary installation (internal)
type Installation struct {
	Path     string
	Source   string
	Version  string
	IsActive bool
}

func findAllInPathWithVersions(name string) []Installation {
	var found []Installation
	pathEnv := os.Getenv("PATH")
	seen := make(map[string]bool)

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		binPath := filepath.Join(dir, name+taskfile.ExeExt())
		if fileExists(binPath) && !seen[binPath] {
			source := "path"
			if strings.Contains(binPath, "homebrew") || strings.Contains(binPath, "Cellar") {
				source = "brew"
			} else if strings.Contains(binPath, "/usr/local/") {
				source = "system"
			}
			version := getToolVersion(binPath, name)
			found = append(found, Installation{
				Path:    binPath,
				Source:  source,
				Version: version,
			})
			seen[binPath] = true
		}
	}

	if runtime.GOOS == "darwin" {
		extraDirs := []string{"/opt/homebrew/bin", "/usr/local/bin"}
		for _, dir := range extraDirs {
			binPath := filepath.Join(dir, name)
			if fileExists(binPath) && !seen[binPath] {
				source := "path"
				if strings.Contains(dir, "homebrew") {
					source = "brew"
				}
				version := getToolVersion(binPath, name)
				found = append(found, Installation{
					Path:    binPath,
					Source:  source,
					Version: version,
				})
				seen[binPath] = true
			}
		}
	}

	return found
}

func getConventionLocations(name string, home string) []string {
	binName := name + taskfile.ExeExt()
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(home, ".bun", "bin", binName),
			filepath.Join(home, "bin", binName),
		}
	}
	return []string{
		filepath.Join(home, ".bun", "bin", name),
		filepath.Join(home, ".local", "bin", name),
	}
}

func findManagedTool(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	tf := findTaskfileForTool(name)
	if tf != nil {
		if path := getInstallPathFromTaskfile(tf, name, home); path != "" {
			if fileExists(path) {
				return path
			}
		}
	}
	return checkConventionLocations(name, home)
}

func findTaskfileForTool(name string) *taskfile.Taskfile {
	root := getRepoRoot()
	if root == "" {
		root = "." // Fallback to cwd
	}

	searchPaths := []string{
		filepath.Join(root, fmt.Sprintf("taskfiles/tools/Taskfile.%s.yml", name)),
		filepath.Join(root, fmt.Sprintf("taskfiles/Taskfile.%s.yml", name)),
		filepath.Join(root, fmt.Sprintf("taskfiles/toolchain/Taskfile.%s.yml", name)),
		filepath.Join(root, fmt.Sprintf("Taskfile.%s.yml", name)),
	}
	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			tf, err := taskfile.Parse(path)
			if err == nil {
				return tf
			}
		}
	}
	return nil
}

func getInstallPathFromTaskfile(tf *taskfile.Taskfile, toolName string, home string) string {
	binName := getBinaryName(tf, toolName)
	if bunBin := tf.GetVarString("BUN_GLOBAL_BIN"); bunBin != "" {
		bunBin = expandHome(bunBin, home)
		return filepath.Join(bunBin, binName)
	}
	upperName := strings.ToUpper(toolName)
	if installDir := tf.GetVarString(upperName + "_INSTALL_DIR"); installDir != "" {
		installDir = expandHome(installDir, home)
		return filepath.Join(installDir, binName)
	}
	if installDir := tf.GetVarString("BIN_INSTALL_DIR"); installDir != "" {
		installDir = expandHome(installDir, home)
		return filepath.Join(installDir, binName)
	}
	return filepath.Join(getDefaultBinInstallDir(home), binName)
}

func getBinaryName(tf *taskfile.Taskfile, toolName string) string {
	binName := taskfile.ExtractBinaryName(tf)
	if binName != "" {
		return binName
	}
	return toolName + taskfile.ExeExt()
}

func checkConventionLocations(name string, home string) string {
	for _, loc := range getConventionLocations(name, home) {
		if fileExists(loc) {
			return loc
		}
	}
	return ""
}

func expandHome(path string, home string) string {
	path = strings.ReplaceAll(path, "{{.HOME}}", home)
	path = strings.ReplaceAll(path, "$HOME", home)
	path = strings.ReplaceAll(path, "~", home)
	return path
}

func getDefaultBinInstallDir(home string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "bin")
	}
	return filepath.Join(home, ".local", "bin")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// getRepoRoot returns repo root or empty string (uses findRepoRoot from release.go)
func getRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, err := findRepoRoot(cwd)
	if err != nil {
		return ""
	}
	return root
}

// getExpectedVersionFromEnv reads TOOL_VERSION from xplat.env
// This is the source of truth for expected versions (Taskfiles use {{.TOOL_VERSION}})
func getExpectedVersionFromEnv(toolName string) string {
	root := getRepoRoot()
	if root == "" {
		return ""
	}
	envPath := filepath.Join(root, "xplat.env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return ""
	}

	// Handle tool name variations: wrangler -> WRANGLER, dummy-cgo -> DUMMY_CGO
	upperName := strings.ToUpper(strings.ReplaceAll(toolName, "-", "_"))
	pattern := upperName + "_VERSION="

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, pattern) {
			return strings.TrimPrefix(line, pattern)
		}
	}
	return ""
}
