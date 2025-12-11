// translate provides translation workflow management for Hugo multilingual content.
//
// Usage:
//
//	translate content status          Show what English files changed since last translation
//	translate content diff <file>     Show git diff for specific file since checkpoint
//	translate content changed         Show detailed changes for all files
//	translate content next            Show next file to translate with progress
//	translate content done            Mark translations complete (update checkpoint)
//	translate content missing         Show files missing in target languages
//	translate content orphans         Show target files with no English source
//	translate content stale           Show potentially outdated translations
//	translate content clean           Delete orphaned files (prompts unless -force)
//
//	translate menu check              Validate menu files for broken links and sync issues
//	translate menu sync               Generate translated menu files from English
//
//	translate lang list               Show configured languages and detect stray directories
//	translate lang validate           Check translator config matches Hugo config
//	translate lang add <code> <name> <dirname>   Add a new target language
//	translate lang remove <code>      Remove a language (prompts unless -force)
//	translate lang init <code>        Initialize content directory for configured language
//
// Flags:
//
//	-github-issue    Output markdown for GitHub Issue (exit 1 if action needed)
//	-force           Skip confirmation prompts (for CI)
//	-version         Print version and exit
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/xplat/internal/translator"
)

// version is set via ldflags at build time
var version = "dev"

func main() {
	// Global flags
	githubIssue := flag.Bool("github-issue", false, "Output markdown for GitHub Issue")
	force := flag.Bool("force", false, "Skip confirmation prompts (for CI)")
	ver := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *ver {
		fmt.Printf("translate %s\n", version)
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	// Create checker instance
	checker, err := translator.NewChecker()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	namespace := flag.Arg(0)
	subCmd := flag.Arg(1)
	var exitCode int

	switch namespace {
	case "content":
		exitCode = runContentCommand(checker, subCmd, *githubIssue, *force)
	case "menu":
		exitCode = runMenuCommand(checker, subCmd, *githubIssue)
	case "lang":
		exitCode = runLangCommand(checker, subCmd, *force)
	default:
		fmt.Fprintf(os.Stderr, "Unknown namespace: %s\n", namespace)
		fmt.Fprintf(os.Stderr, "Available: content, menu, lang\n")
		usage()
		os.Exit(1)
	}

	os.Exit(exitCode)
}

// ============================================================================
// Content Commands - Track English source changes and translation problems
// ============================================================================

func runContentCommand(c *translator.Checker, subCmd string, githubIssue, force bool) int {
	switch subCmd {
	case "status":
		return runStatus(c, githubIssue)
	case "diff":
		file := flag.Arg(2)
		if file == "" {
			fmt.Fprintln(os.Stderr, "Error: content diff requires a file argument")
			fmt.Fprintln(os.Stderr, "Usage: translate content diff <file>")
			return 1
		}
		return runDiff(c, file)
	case "changed":
		return runChanged(c)
	case "next":
		return runNext(c)
	case "done":
		return runDone(c)
	case "missing":
		return runMissing(c, githubIssue)
	case "orphans":
		return runOrphans(c, githubIssue)
	case "stale":
		return runStale(c, githubIssue)
	case "clean":
		return runClean(c, force)
	case "":
		fmt.Fprintln(os.Stderr, "Error: content requires a subcommand")
		contentUsage()
		return 1
	default:
		fmt.Fprintf(os.Stderr, "Unknown content command: %s\n", subCmd)
		contentUsage()
		return 1
	}
}

// ============================================================================
// Menu Commands - Navigation menu management
// ============================================================================

func runMenuCommand(c *translator.Checker, subCmd string, githubIssue bool) int {
	switch subCmd {
	case "check":
		return runMenuCheck(c, githubIssue)
	case "sync":
		return runMenuSync(c)
	case "":
		fmt.Fprintln(os.Stderr, "Error: menu requires a subcommand")
		menuUsage()
		return 1
	default:
		fmt.Fprintf(os.Stderr, "Unknown menu command: %s\n", subCmd)
		menuUsage()
		return 1
	}
}

// ============================================================================
// Lang Commands - Language management
// ============================================================================

func runLangCommand(c *translator.Checker, subCmd string, force bool) int {
	switch subCmd {
	case "list":
		return runLangs(c)
	case "validate":
		return runValidate(c)
	case "add":
		return runLangAdd(c)
	case "remove":
		return runLangRemove(c, force)
	case "init":
		return runLangInit(c)
	case "":
		fmt.Fprintln(os.Stderr, "Error: lang requires a subcommand")
		langUsage()
		return 1
	default:
		fmt.Fprintf(os.Stderr, "Unknown lang command: %s\n", subCmd)
		langUsage()
		return 1
	}
}

// ============================================================================
// Run Functions - Wire checker functions to presenters
// ============================================================================

func runStatus(c *translator.Checker, githubIssue bool) int {
	result := c.CheckStatus()

	if githubIssue {
		p := translator.NewMarkdownPresenter()
		p.Status(result)
		if result.HasIssues() {
			return 1
		}
		return 0
	}

	p := translator.NewTerminalPresenter()
	p.Status(result)
	return 0
}

func runDiff(c *translator.Checker, file string) int {
	result := c.CheckDiff(file)

	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "ERROR: File not found: %s\n", file)
		return 1
	}

	p := translator.NewTerminalPresenter()
	p.Diff(result)
	return 0
}

func runMissing(c *translator.Checker, githubIssue bool) int {
	result := c.CheckMissing()

	if githubIssue {
		p := translator.NewMarkdownPresenter()
		p.Missing(result)
		if result.HasIssues() {
			return 1
		}
		return 0
	}

	p := translator.NewTerminalPresenter()
	p.Missing(result)
	return 0
}

func runStale(c *translator.Checker, githubIssue bool) int {
	result := c.CheckStale()

	if githubIssue {
		p := translator.NewMarkdownPresenter()
		p.Stale(result)
		if result.HasIssues() {
			return 1
		}
		return 0
	}

	p := translator.NewTerminalPresenter()
	p.Stale(result)
	return 0
}

func runOrphans(c *translator.Checker, githubIssue bool) int {
	result := c.CheckOrphans()

	if githubIssue {
		p := translator.NewMarkdownPresenter()
		p.Orphans(result)
		if result.HasIssues() {
			return 1
		}
		return 0
	}

	p := translator.NewTerminalPresenter()
	p.Orphans(result)
	return 0
}

func runClean(c *translator.Checker, force bool) int {
	// First pass: get what would be deleted
	result := c.DoClean(force, false)

	if result.TotalCount == 0 {
		fmt.Println("OK: No orphaned files to delete")
		return 0
	}

	// Show what will be deleted
	p := translator.NewTerminalPresenter()
	p.Clean(result)

	// If force, skip confirmation
	if !force {
		fmt.Print("\nDelete these files? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(response)
		if response != "y" && response != "Y" {
			fmt.Println("Cancelled")
			return 0
		}
	}

	// Second pass: actually delete
	result = c.DoClean(force, true)
	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", result.Error)
		return 1
	}

	fmt.Printf("\nOK: Deleted %d orphaned files\n", result.TotalCount)
	return 0
}

func runDone(c *translator.Checker) int {
	result := c.DoDone()

	p := translator.NewTerminalPresenter()
	p.Done(result)

	if result.Error != nil {
		return 1
	}
	return 0
}

func runNext(c *translator.Checker) int {
	result := c.CheckNext()

	p := translator.NewTerminalPresenter()
	p.Next(result)
	return 0
}

func runChanged(c *translator.Checker) int {
	result := c.CheckChanged()

	p := translator.NewTerminalPresenter()
	p.Changed(result)
	return 0
}

func runValidate(c *translator.Checker) int {
	result := c.CheckValidate()
	config := c.GetConfig()

	// For validate, we need more context in output
	fmt.Println("========================================")
	fmt.Println("Validating Translator Configuration")
	fmt.Println("========================================")
	fmt.Println()

	// Check if this is a Hugo project
	if !translator.IsHugoProject() {
		fmt.Println("Mode: Standalone (no Hugo config found)")
		fmt.Println()
		fmt.Println("Current configuration:")
		fmt.Printf("  Source: %s -> content/%s\n", config.SourceLang, config.SourceDir)
		for _, lang := range config.TargetLangs {
			fmt.Printf("  Target: %s (%s) -> content/%s\n", lang.Code, lang.Name, lang.DirName)
		}
		fmt.Println()
		fmt.Println("========================================")
		fmt.Println("OK: Using default configuration")
		fmt.Println("========================================")
		return 0
	}

	fmt.Println("Mode: Hugo project detected")
	fmt.Println()

	// Show current config (auto-loaded from Hugo)
	fmt.Printf("Source: %s -> content/%s\n", config.SourceLang, config.SourceDir)
	for _, lang := range config.TargetLangs {
		fmt.Printf("Target: %s (%s) -> content/%s\n", lang.Code, lang.Name, lang.DirName)
	}
	fmt.Println()

	if result.HasIssues() {
		fmt.Println("========================================")
		fmt.Printf("WARNING: %d mismatch(es) found\n", len(result.Mismatches))
		fmt.Println("========================================")
		for _, m := range result.Mismatches {
			fmt.Printf("  - %s\n", m)
		}
		fmt.Println()
		fmt.Println("This shouldn't happen - languages are auto-loaded from Hugo config.")
		fmt.Println("Check if config/_default/languages.toml changed after binary was built.")
		return 1
	}

	fmt.Println("========================================")
	fmt.Println("OK: Configuration loaded from Hugo")
	fmt.Println("========================================")
	return 0
}

func runLangs(c *translator.Checker) int {
	result := c.CheckLangs()

	p := translator.NewTerminalPresenter()
	p.Langs(result)

	if result.HasIssues() {
		return 1
	}
	return 0
}

func runMenuCheck(c *translator.Checker, githubIssue bool) int {
	result := c.CheckMenu()

	if githubIssue {
		p := translator.NewMarkdownPresenter()
		p.MenuCheck(result)
		if result.HasIssues() {
			return 1
		}
		return 0
	}

	p := translator.NewTerminalPresenter()
	p.MenuCheck(result)
	return 0
}

func runMenuSync(c *translator.Checker) int {
	// Show header with source info
	enMenuPath := translator.GetMenuFilePath("en")
	enMenu, err := translator.ParseMenuFile(enMenuPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading English menu: %v\n", err)
		return 1
	}

	fmt.Println("========================================")
	fmt.Println("Menu Sync")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Printf("Source: %s (%d main items, %d footer items)\n", enMenuPath, len(enMenu.Main), len(enMenu.Footer))
	fmt.Println()

	result := c.DoMenuSync()

	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", result.Error)
		return 1
	}

	for _, path := range result.FilesWritten {
		fmt.Printf("Generated: %s\n", path)
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("OK: Menu files regenerated from English")
	fmt.Println("========================================")
	return 0
}

// ============================================================================
// Language Management Implementations
// ============================================================================

func runLangAdd(c *translator.Checker) int {
	if flag.NArg() < 5 {
		fmt.Fprintln(os.Stderr, "Error: lang add requires <code> <name> <dirname>")
		fmt.Fprintln(os.Stderr, "Usage: translate lang add fr \"Francais\" french")
		return 1
	}

	code := flag.Arg(2)
	name := flag.Arg(3)
	dirname := flag.Arg(4)

	fmt.Println("========================================")
	fmt.Printf("Adding language: %s (%s)\n", code, name)
	fmt.Println("========================================")
	fmt.Println()

	result := c.DoLangAdd(code, name, dirname)

	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", result.Error)
		return 1
	}

	fmt.Printf("1. Adding to %s...\n", result.ConfigPath)
	fmt.Println("   OK")
	fmt.Printf("2. Creating %s...\n", result.ContentPath)
	fmt.Println("   OK")
	if result.MenuPath != "" {
		fmt.Printf("3. Generating %s...\n", result.MenuPath)
		fmt.Println("   OK")
	}

	fmt.Println()
	fmt.Println("========================================")
	fmt.Printf("OK: Language '%s' added\n", code)
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Run 'task translate:content:missing' to see what needs translating\n")
	fmt.Printf("  2. Translate content to %s\n", name)
	fmt.Printf("  3. Run 'task translate:content:done' when complete\n")
	return 0
}

func runLangRemove(c *translator.Checker, force bool) int {
	if flag.NArg() < 3 {
		fmt.Fprintln(os.Stderr, "Error: lang remove requires <code>")
		fmt.Fprintln(os.Stderr, "Usage: translate lang remove fr [-force]")
		return 1
	}

	code := flag.Arg(2)
	config := c.GetConfig()

	fmt.Println("========================================")
	fmt.Printf("Removing language: %s\n", code)
	fmt.Println("========================================")
	fmt.Println()

	// Check if language exists
	existing, err := translator.GetLanguageByCode(code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		return 1
	}
	if existing == nil {
		fmt.Printf("Language '%s' not found in config\n", code)
		return 1
	}

	// Get content directory
	dirname := strings.TrimPrefix(existing.ContentDir, "content/")
	contentPath := filepath.Join(config.ContentDir, dirname)

	// Count files in content directory
	fileCount := 0
	if _, err := os.Stat(contentPath); err == nil {
		filepath.Walk(contentPath, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
				fileCount++
			}
			return nil
		})
	}

	// Show what will be deleted
	fmt.Printf("Content directory: %s (%d .md files)\n", contentPath, fileCount)
	fmt.Printf("Config: config/_default/languages.toml [%s] section\n", code)
	fmt.Printf("Menu: config/_default/menus.%s.toml\n", code)
	fmt.Println()

	// Confirm unless force
	if !force && fileCount > 0 {
		fmt.Printf("Delete %d files and remove language '%s'? [y/N]: ", fileCount, code)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(response)
		if response != "y" && response != "Y" {
			fmt.Println("Cancelled")
			return 0
		}
	}

	result := c.DoLangRemove(code, force, true)

	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", result.Error)
		return 1
	}

	fmt.Println("1. Removing from languages.toml...")
	fmt.Println("   OK")
	if result.FilesRemoved > 0 {
		fmt.Printf("2. Deleting %s...\n", contentPath)
		fmt.Println("   OK")
	}
	fmt.Printf("3. Deleting menus.%s.toml...\n", code)
	fmt.Println("   OK")

	fmt.Println()
	fmt.Println("========================================")
	fmt.Printf("OK: Language '%s' removed\n", code)
	fmt.Println("========================================")
	return 0
}

func runLangInit(c *translator.Checker) int {
	if flag.NArg() < 3 {
		fmt.Fprintln(os.Stderr, "Error: lang init requires <code>")
		fmt.Fprintln(os.Stderr, "Usage: translate lang init fr")
		return 1
	}

	code := flag.Arg(2)

	fmt.Println("========================================")
	fmt.Printf("Initializing content for language: %s\n", code)
	fmt.Println("========================================")
	fmt.Println()

	result := c.DoLangInit(code)

	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", result.Error)
		return 1
	}

	if result.AlreadyExists {
		fmt.Printf("Directory %s already exists (%d .md files)\n", result.Path, result.FileCount)
		fmt.Println("========================================")
		return 0
	}

	fmt.Printf("Creating %s...\n", result.Path)
	fmt.Println("OK")

	fmt.Println()
	fmt.Println("========================================")
	fmt.Printf("OK: Content directory initialized for '%s'\n", code)
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  Run 'task translate:content:missing' to see what needs translating\n")
	return 0
}

// ============================================================================
// Usage
// ============================================================================

func usage() {
	fmt.Fprintf(os.Stderr, `translate - Translation workflow for Hugo multilingual content

Usage:
  translate <namespace> <command> [args] [flags]

Namespaces:
  content   Track English source changes and find translation problems
  menu      Manage navigation menus per language
  lang      Add, remove, and configure languages

Flags:
  -github-issue  Output markdown for GitHub Issue (exit 1 if action needed)
  -force         Skip confirmation prompts (for CI)
  -version       Print version and exit

Run 'translate <namespace>' for namespace-specific help.

Examples:
  translate content status              # See what English files changed
  translate content missing             # See what's missing in translations
  translate menu check                  # Validate menu files
  translate lang list                   # Show configured languages
  translate lang add fr Francais french # Add French language

`)
}

func contentUsage() {
	fmt.Fprintf(os.Stderr, `translate content - Track English source changes and translation problems

Commands:
  status            Show what English files changed since last translation
  diff <file>       Show git diff for specific file since checkpoint
  changed           Show detailed changes for all files
  next              Show next file to translate with progress
  done              Mark translations complete (update checkpoint)
  missing           Show files missing in target languages
  orphans           Show target files with no English source
  stale             Show potentially outdated translations (target < 50%% of source)
  clean             Delete orphaned files (prompts unless -force)

Examples:
  translate content status
  translate content diff blog/my-post.md
  translate content missing -github-issue
  translate content clean -force

`)
}

func menuUsage() {
	fmt.Fprintf(os.Stderr, `translate menu - Manage navigation menus per language

Commands:
  check             Validate menu files for broken links and sync issues
  sync              Generate translated menu files from English

Examples:
  translate menu check
  translate menu check -github-issue
  translate menu sync

`)
}

func langUsage() {
	fmt.Fprintf(os.Stderr, `translate lang - Add, remove, and configure languages

Commands:
  list              Show configured languages and detect stray directories
  validate          Check translator config matches Hugo config
  add <code> <name> <dirname>   Add a new target language
  remove <code>     Remove a language (prompts unless -force)
  init <code>       Initialize content directory for configured language

Examples:
  translate lang list
  translate lang add fr "Francais" french
  translate lang add ko "Korean" korean
  translate lang remove fr
  translate lang remove fr -force
  translate lang init fr

`)
}
