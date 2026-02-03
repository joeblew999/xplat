// Package cmd provides CLI commands for xplat.
//
// docs.go - Generate documentation from xplat commands
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/templates"
	"github.com/spf13/cobra"
)

// DocsCmd generates documentation from xplat commands.
// This is an INTERNAL command for xplat developers only.
// Users of plat-* repos should use 'xplat gen' instead.
var DocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate xplat's own documentation (for xplat developers)",
	Long: `Generate xplat's own README.md and Taskfile.yml from its command structure.

This is for xplat developers only - NOT for plat-* repos.
If you're working in a plat-* repo, use 'xplat gen all' instead.

Examples:
  xplat internal docs all         # Generate README.md + Taskfile.yml
  xplat internal docs readme      # Generate README.md only
  xplat internal docs taskfile    # Generate Taskfile.yml only`,
}

var docsReadmeCmd = &cobra.Command{
	Use:   "readme",
	Short: "Generate README.md from xplat commands",
	RunE:  runDocsReadme,
}

var docsTaskfileCmd = &cobra.Command{
	Use:   "taskfile",
	Short: "Generate Taskfile.yml command wrappers",
	RunE:  runDocsTaskfile,
}

var docsAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Generate all documentation (README.md + Taskfile.yml + docs/*.md)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runDocsReadme(cmd, args); err != nil {
			return err
		}
		if err := runDocsCLI(cmd, args); err != nil {
			return err
		}
		if err := runDocsConfig(cmd, args); err != nil {
			return err
		}
		if err := runDocsChangelog(cmd, args); err != nil {
			return err
		}
		return runDocsTaskfile(cmd, args)
	},
}

var docsCLICmd = &cobra.Command{
	Use:   "cli",
	Short: "Generate docs/CLI.md with full command reference",
	RunE:  runDocsCLI,
}

var docsConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate docs/CONFIG.md from internal/config/config.go",
	RunE:  runDocsConfig,
}

var docsChangelogCmd = &cobra.Command{
	Use:   "changelog",
	Short: "Generate docs/CHANGELOG.md from git history",
	RunE:  runDocsChangelog,
}

var docsOutputDir string

func init() {
	DocsCmd.PersistentFlags().StringVarP(&docsOutputDir, "output", "o", ".", "Output directory")

	DocsCmd.AddCommand(docsReadmeCmd)
	DocsCmd.AddCommand(docsTaskfileCmd)
	DocsCmd.AddCommand(docsCLICmd)
	DocsCmd.AddCommand(docsConfigCmd)
	DocsCmd.AddCommand(docsChangelogCmd)
	DocsCmd.AddCommand(docsAllCmd)
}

// extractCommands extracts command info from a cobra command tree.
func extractCommands(cmd *cobra.Command) []templates.CommandInfo {
	var commands []templates.CommandInfo

	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		info := templates.CommandInfo{
			Name:  sub.Name(),
			Short: sub.Short,
			Long:  sub.Long,
			Use:   sub.Use,
		}
		// Get subcommands
		for _, subsub := range sub.Commands() {
			if !subsub.Hidden {
				info.Subcommands = append(info.Subcommands, templates.CommandInfo{
					Name:  subsub.Name(),
					Short: subsub.Short,
					Use:   subsub.Use,
				})
			}
		}
		commands = append(commands, info)
	}

	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	return commands
}

func runDocsReadme(cmd *cobra.Command, args []string) error {
	root := cmd.Root()
	commands := extractCommands(root)

	// Group commands by category
	categoryMap := map[string][]templates.CommandInfo{
		"Core":               {},
		"Package Management": {},
		"Process":            {},
		"Sync":               {},
		"Development":        {},
		"Other":              {},
	}

	for _, c := range commands {
		switch c.Name {
		case "task", "process", "gen", "manifest", "run", "version", "update":
			categoryMap["Core"] = append(categoryMap["Core"], c)
		case "pkg", "binary":
			categoryMap["Package Management"] = append(categoryMap["Package Management"], c)
		case "service", "release":
			categoryMap["Process"] = append(categoryMap["Process"], c)
		case "sync-gh", "sync-cf":
			categoryMap["Sync"] = append(categoryMap["Sync"], c)
		case "docs", "os", "completion":
			categoryMap["Development"] = append(categoryMap["Development"], c)
		default:
			categoryMap["Other"] = append(categoryMap["Other"], c)
		}
	}

	// Category descriptions
	categoryDescriptions := map[string]string{
		"Core":               "",
		"Package Management": "",
		"Process":            "",
		"Sync":               "Monitor GitHub and Cloudflare for events (releases, CI, deploys). See [Sync Commands](#sync-commands) above.",
		"Development":        "",
		"Other":              "",
	}

	// Build categories in order
	categoryOrder := []string{"Core", "Package Management", "Process", "Sync", "Development", "Other"}
	var categories []templates.CommandCategory
	for _, name := range categoryOrder {
		cmds := categoryMap[name]
		if len(cmds) > 0 {
			categories = append(categories, templates.CommandCategory{
				Name:        name,
				Description: categoryDescriptions[name],
				Commands:    cmds,
			})
		}
	}

	// Render template
	data := templates.XplatReadmeData{
		Categories:  categories,
		AllCommands: commands,
	}

	content, err := templates.RenderInternal("readme.xplat.md.tmpl", data)
	if err != nil {
		return fmt.Errorf("failed to render README: %w", err)
	}

	// Write file
	outPath := filepath.Join(docsOutputDir, "README.md")
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}

func runDocsTaskfile(cmd *cobra.Command, args []string) error {
	root := cmd.Root()

	// Extract commands for wrappers (skip hidden, completion, help)
	var commands []templates.CommandInfo
	for _, sub := range root.Commands() {
		if sub.Hidden {
			continue
		}
		name := sub.Name()
		if name == "completion" || name == "help" {
			continue
		}
		commands = append(commands, templates.CommandInfo{
			Name:  name,
			Short: sub.Short,
		})
	}

	// Render template
	data := templates.XplatTaskfileData{
		Commands: commands,
	}

	content, err := templates.RenderInternal("taskfile.xplat.yml.tmpl", data)
	if err != nil {
		return fmt.Errorf("failed to render Taskfile: %w", err)
	}

	// Write file
	outPath := filepath.Join(docsOutputDir, "Taskfile.yml")
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}

func runDocsCLI(cmd *cobra.Command, args []string) error {
	root := cmd.Root()

	// Ensure docs directory exists
	docsDir := filepath.Join(docsOutputDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return fmt.Errorf("failed to create docs dir: %w", err)
	}

	// Build CLI reference content
	var content strings.Builder
	content.WriteString("# CLI Reference\n\n")
	content.WriteString("Complete reference for all xplat commands.\n\n")
	content.WriteString("*Auto-generated by `xplat internal docs cli`*\n\n")

	// Group commands by category
	categoryMap := map[string][]*cobra.Command{
		"Core":               {},
		"Package Management": {},
		"Process":            {},
		"Sync":               {},
		"Development":        {},
		"Other":              {},
	}

	for _, sub := range root.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		switch sub.Name() {
		case "task", "process", "gen", "manifest", "run", "version", "update", "up":
			categoryMap["Core"] = append(categoryMap["Core"], sub)
		case "pkg", "binary":
			categoryMap["Package Management"] = append(categoryMap["Package Management"], sub)
		case "service", "release":
			categoryMap["Process"] = append(categoryMap["Process"], sub)
		case "sync-gh", "sync-cf":
			categoryMap["Sync"] = append(categoryMap["Sync"], sub)
		case "docs", "os":
			categoryMap["Development"] = append(categoryMap["Development"], sub)
		default:
			categoryMap["Other"] = append(categoryMap["Other"], sub)
		}
	}

	// Write each category
	categoryOrder := []string{"Core", "Package Management", "Process", "Sync", "Development", "Other"}
	for _, cat := range categoryOrder {
		cmds := categoryMap[cat]
		if len(cmds) == 0 {
			continue
		}

		content.WriteString(fmt.Sprintf("## %s\n\n", cat))

		for _, c := range cmds {
			content.WriteString(fmt.Sprintf("### `xplat %s`\n\n", c.Name()))
			content.WriteString(fmt.Sprintf("%s\n\n", c.Short))

			if c.Long != "" && c.Long != c.Short {
				content.WriteString(fmt.Sprintf("```\n%s\n```\n\n", c.Long))
			}

			// List subcommands
			subs := c.Commands()
			if len(subs) > 0 {
				content.WriteString("**Subcommands:**\n\n")
				content.WriteString("| Command | Description |\n")
				content.WriteString("|---------|-------------|\n")
				for _, sub := range subs {
					if !sub.Hidden {
						content.WriteString(fmt.Sprintf("| `%s %s` | %s |\n", c.Name(), sub.Name(), sub.Short))
					}
				}
				content.WriteString("\n")
			}
		}
	}

	// Write file
	outPath := filepath.Join(docsDir, "CLI.md")
	if err := os.WriteFile(outPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}

func runDocsConfig(_ *cobra.Command, _ []string) error {
	// Ensure docs directory exists
	docsDir := filepath.Join(docsOutputDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return fmt.Errorf("failed to create docs dir: %w", err)
	}

	// Build CONFIG.md content
	var content strings.Builder
	content.WriteString("# Configuration Reference\n\n")
	content.WriteString("Configuration constants and environment variables for xplat.\n\n")
	content.WriteString("*Auto-generated by `xplat internal docs config`*\n\n")

	// Ports section
	content.WriteString("## Port Allocation\n\n")
	content.WriteString("xplat uses the 876x port range for its services:\n\n")
	content.WriteString("| Port | Service | Constant |\n")
	content.WriteString("|------|---------|----------|\n")
	content.WriteString(fmt.Sprintf("| %s | Web UI | `DefaultUIPort` |\n", config.DefaultUIPort))
	content.WriteString(fmt.Sprintf("| %d | Process Compose API | `DefaultProcessComposePort` |\n", config.DefaultProcessComposePort))
	content.WriteString(fmt.Sprintf("| %s | MCP HTTP Server | `DefaultMCPPort` |\n", config.DefaultMCPPort))
	content.WriteString(fmt.Sprintf("| %s | Webhook Server | `DefaultWebhookPort` |\n", config.DefaultWebhookPort))
	content.WriteString(fmt.Sprintf("| %s | Docs Server | `DefaultDocsPort` |\n", config.DefaultDocsPort))
	content.WriteString("\n")

	// Environment variables section
	content.WriteString("## Environment Variables\n\n")
	content.WriteString("### Global xplat Home\n\n")
	content.WriteString("| Variable | Default | Description |\n")
	content.WriteString("|----------|---------|-------------|\n")
	content.WriteString("| `XPLAT_HOME` | `~/.xplat` | Global xplat home directory |\n")
	content.WriteString("\n")

	content.WriteString("### Project-Local Directories\n\n")
	content.WriteString("| Variable | Default | Description |\n")
	content.WriteString("|----------|---------|-------------|\n")
	content.WriteString("| `PLAT_SRC` | `.src/` | Project source directory (cloned upstream code) |\n")
	content.WriteString("| `PLAT_BIN` | `.bin/` | Project binary directory (built/downloaded binaries) |\n")
	content.WriteString("| `PLAT_DATA` | `.data/` | Project data directory (databases, caches, logs) |\n")
	content.WriteString("| `PLAT_DIST` | `.dist/` | Project dist directory (release artifacts) |\n")
	content.WriteString("\n")

	// Directory structure section
	content.WriteString("## Directory Structure\n\n")
	content.WriteString("### Global Directories (`~/.xplat/`)\n\n")
	content.WriteString("```\n")
	content.WriteString("~/.xplat/\n")
	content.WriteString("├── bin/           # Global binaries (cross-project)\n")
	content.WriteString("├── cache/         # Downloaded taskfiles, package cache\n")
	content.WriteString("├── config/        # User preferences, credentials\n")
	content.WriteString("└── projects.yaml  # Local project registry\n")
	content.WriteString("```\n\n")

	content.WriteString("### Project Directories\n\n")
	content.WriteString("```\n")
	content.WriteString("plat-myproject/\n")
	content.WriteString("├── .src/          # Cloned upstream source code\n")
	content.WriteString("├── .bin/          # Built or downloaded binaries\n")
	content.WriteString("├── .data/         # Runtime data (databases, logs)\n")
	content.WriteString("├── .dist/         # Release artifacts\n")
	content.WriteString("├── xplat.yaml     # Project manifest\n")
	content.WriteString("└── Taskfile.yml   # Task definitions\n")
	content.WriteString("```\n\n")

	// Task defaults section
	content.WriteString("## Task Runner Defaults\n\n")
	defaults := config.GetTaskDefaults()
	content.WriteString("| Setting | Value | Description |\n")
	content.WriteString("|---------|-------|-------------|\n")
	content.WriteString(fmt.Sprintf("| Trusted Hosts | `%s` | Hosts that skip confirmation prompts |\n", strings.Join(defaults.TrustedHosts, "`, `")))
	content.WriteString(fmt.Sprintf("| Cache Expiry | `%s` | How long to cache remote taskfiles |\n", defaults.CacheExpiryDuration))
	content.WriteString(fmt.Sprintf("| Timeout | `%s` | Timeout for downloading remote taskfiles |\n", defaults.Timeout))
	content.WriteString(fmt.Sprintf("| Failfast | `%t` | Stop on first failure in parallel tasks |\n", defaults.Failfast))
	content.WriteString("\n")

	// File paths section
	content.WriteString("## Default File Paths\n\n")
	content.WriteString("| Constant | Value | Description |\n")
	content.WriteString("|----------|-------|-------------|\n")
	content.WriteString(fmt.Sprintf("| `DefaultTaskfile` | `%s` | Default Taskfile path |\n", config.DefaultTaskfile))
	content.WriteString(fmt.Sprintf("| `ProcessComposeGeneratedFile` | `%s` | Generated process-compose config |\n", config.ProcessComposeGeneratedFile))
	content.WriteString("\n")

	content.WriteString("### Process Compose Search Order\n\n")
	content.WriteString("When looking for process-compose config, xplat searches in this order:\n\n")
	for i, f := range config.ProcessComposeSearchOrder() {
		content.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, f))
	}
	content.WriteString("\n")

	// Write file
	outPath := filepath.Join(docsDir, "CONFIG.md")
	if err := os.WriteFile(outPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}

func runDocsChangelog(_ *cobra.Command, _ []string) error {
	// Ensure docs directory exists
	docsDir := filepath.Join(docsOutputDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		return fmt.Errorf("failed to create docs dir: %w", err)
	}

	// Get git log
	cmd := exec.Command("git", "log", "--pretty=format:%H|%ad|%s", "--date=short", "-100")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get git log: %w", err)
	}

	// Parse commits and group by date
	type commit struct {
		hash    string
		date    string
		message string
	}
	var commits []commit
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		commits = append(commits, commit{
			hash:    parts[0][:7], // Short hash
			date:    parts[1],
			message: parts[2],
		})
	}

	// Build CHANGELOG.md content
	var content strings.Builder
	content.WriteString("# Changelog\n\n")
	content.WriteString("Recent changes to xplat.\n\n")
	content.WriteString(fmt.Sprintf("*Auto-generated by `xplat internal docs changelog` on %s*\n\n", time.Now().Format("2006-01-02")))

	// Group by date
	currentDate := ""
	tagPattern := regexp.MustCompile(`^v?\d+\.\d+`)

	for _, c := range commits {
		// Check if this looks like a release (version tag in message)
		isRelease := tagPattern.MatchString(c.message) || strings.HasPrefix(c.message, "Release") || strings.HasPrefix(c.message, "release")

		if c.date != currentDate {
			if currentDate != "" {
				content.WriteString("\n")
			}
			content.WriteString(fmt.Sprintf("## %s\n\n", c.date))
			currentDate = c.date
		}

		// Format: bullet with commit message, hash link
		prefix := "-"
		if isRelease {
			prefix = "###"
			content.WriteString(fmt.Sprintf("%s %s\n\n", prefix, c.message))
		} else {
			content.WriteString(fmt.Sprintf("%s %s (`%s`)\n", prefix, c.message, c.hash))
		}
	}

	// Write file
	outPath := filepath.Join(docsDir, "CHANGELOG.md")
	if err := os.WriteFile(outPath, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outPath, err)
	}

	fmt.Printf("Generated %s\n", outPath)
	return nil
}
