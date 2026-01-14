// Package cmd provides CLI commands for xplat.
//
// docs.go - Generate documentation from xplat commands
package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// DocsCmd generates documentation from xplat commands.
var DocsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate documentation from xplat commands",
	Long: `Generate README.md and Taskfile.yml from xplat's command structure.

This keeps documentation in sync with the actual code.

Examples:
  xplat docs readme      # Generate README.md
  xplat docs taskfile    # Generate Taskfile.yml with xplat tasks
  xplat docs all         # Generate both`,
}

var docsReadmeCmd = &cobra.Command{
	Use:   "readme",
	Short: "Generate README.generated.md from xplat commands",
	RunE:  runDocsReadme,
}

var docsTaskfileCmd = &cobra.Command{
	Use:   "taskfile",
	Short: "Generate Taskfile.generated.yml with xplat wrapper tasks",
	RunE:  runDocsTaskfile,
}

var docsAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Generate all documentation",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runDocsReadme(cmd, args); err != nil {
			return err
		}
		return runDocsTaskfile(cmd, args)
	},
}

var docsOutputDir string

func init() {
	DocsCmd.PersistentFlags().StringVarP(&docsOutputDir, "output", "o", ".", "Output directory")

	DocsCmd.AddCommand(docsReadmeCmd)
	DocsCmd.AddCommand(docsTaskfileCmd)
	DocsCmd.AddCommand(docsAllCmd)
}

// commandInfo holds extracted command metadata
type commandInfo struct {
	Name        string
	Short       string
	Long        string
	Use         string
	Subcommands []commandInfo
}

// extractCommands extracts command info from a cobra command tree
func extractCommands(cmd *cobra.Command) []commandInfo {
	var commands []commandInfo

	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		info := commandInfo{
			Name:  sub.Name(),
			Short: sub.Short,
			Long:  sub.Long,
			Use:   sub.Use,
		}
		// Get subcommands
		for _, subsub := range sub.Commands() {
			if !subsub.Hidden {
				info.Subcommands = append(info.Subcommands, commandInfo{
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

	var sb strings.Builder

	sb.WriteString("# xplat\n\n")
	sb.WriteString("**One binary to bootstrap and run any plat-* project.**\n\n")

	sb.WriteString("## Why?\n\n")
	sb.WriteString("Instead of installing Task, process-compose, and various CLIs separately,\n")
	sb.WriteString("xplat embeds them all. One binary, works on macOS/Linux/Windows.\n\n")

	sb.WriteString("## ✨ Composability - The Key Feature\n\n")
	sb.WriteString("**Reuse tasks and processes across projects.**\n\n")
	sb.WriteString("xplat enables composability: install packages from other xplat projects\n")
	sb.WriteString("and immediately use their tasks and processes in your project.\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# Install a package\n")
	sb.WriteString("xplat pkg install plat-nats --with-process\n\n")
	sb.WriteString("# Generate includes from installed packages\n")
	sb.WriteString("xplat gen taskfile    # Creates Taskfile.generated.yml with remote includes\n")
	sb.WriteString("xplat gen process     # Creates pc.generated.yaml with processes\n\n")
	sb.WriteString("# Now use tasks from the installed package!\n")
	sb.WriteString("task nats:run         # Run tasks defined in plat-nats\n")
	sb.WriteString("process-compose up    # Run processes including plat-nats\n")
	sb.WriteString("```\n\n")
	sb.WriteString("**How it works:**\n")
	sb.WriteString("1. `xplat pkg install` downloads binaries and records package info in `xplat-lock.yaml`\n")
	sb.WriteString("2. `xplat gen taskfile` reads the lockfile and generates remote Taskfile includes\n")
	sb.WriteString("3. `xplat gen process` reads the lockfile and generates process-compose definitions\n\n")
	sb.WriteString("This lets you build a platform from composable pieces - each `plat-*` project\n")
	sb.WriteString("can expose tasks and processes that other projects can reuse.\n\n")

	sb.WriteString("## Quick Start\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# 1. Bootstrap a new project\n")
	sb.WriteString("xplat manifest bootstrap\n\n")
	sb.WriteString("# 2. Generate project files from xplat.yaml\n")
	sb.WriteString("xplat gen all\n\n")
	sb.WriteString("# 3. Build/test/lint (embedded Task)\n")
	sb.WriteString("xplat task build\n")
	sb.WriteString("xplat task test\n\n")
	sb.WriteString("# 4. Run services (embedded process-compose)\n")
	sb.WriteString("xplat process\n\n")
	sb.WriteString("# 5. Install packages from registry\n")
	sb.WriteString("xplat pkg install <name>\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Installation\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# Build from source\n")
	sb.WriteString("go build -o xplat .\n\n")
	sb.WriteString("# Or install to ~/.local/bin\n")
	sb.WriteString("go build -o ~/.local/bin/xplat .\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Architecture\n\n")
	sb.WriteString("xplat solves the problem of consistent tooling across multiple `plat-*` projects on Mac/Linux/Windows.\n\n")
	sb.WriteString("| Component | Purpose |\n")
	sb.WriteString("|-----------|--------|\n")
	sb.WriteString("| **Embedded Task** | Declarative build system. Taskfile.yml defines build/test/lint. |\n")
	sb.WriteString("| **Embedded process-compose** | Multi-process orchestration. Run app + dependencies together. |\n")
	sb.WriteString("| **xplat.yaml manifest** | Single source of truth: language, binary, env vars, processes. |\n")
	sb.WriteString("| **gen commands** | Generate CI, .gitignore, .env from manifest. Change manifest, regenerate. |\n")
	sb.WriteString("| **pkg registry** | Shared tooling. Install a package = binary + taskfile + process config. |\n")
	sb.WriteString("| **os utilities** | Cross-platform primitives (rm, cp, glob) that behave identically everywhere. |\n")
	sb.WriteString("| **sync-gh / sync-cf** | Watch external services (GitHub, Cloudflare) for events. No vendor CLI needed. |\n\n")
	sb.WriteString("**The pattern:**\n")
	sb.WriteString("```\n")
	sb.WriteString("xplat.yaml (manifest) → gen → Taskfile.yml, process-compose.yaml, CI workflow\n")
	sb.WriteString("                       ↓\n")
	sb.WriteString("                    xplat task build    (runs tasks)\n")
	sb.WriteString("                    xplat process       (runs services)\n")
	sb.WriteString("```\n\n")
	sb.WriteString("## Sync Commands\n\n")
	sb.WriteString("The `sync-gh` and `sync-cf` commands monitor external services without requiring vendor CLIs.\n\n")
	sb.WriteString("**Why?** You often need to react to external events:\n")
	sb.WriteString("- A dependency released a new version (GitHub release)\n")
	sb.WriteString("- CI workflow completed (GitHub Actions)\n")
	sb.WriteString("- A deploy finished (Cloudflare Pages)\n\n")
	sb.WriteString("**How it works:**\n")
	sb.WriteString("1. **Polling** - Periodically check APIs for changes (`sync-gh poll`, `sync-cf poll`)\n")
	sb.WriteString("2. **Webhooks** - Receive push notifications from services (`sync-gh webhook`, `sync-cf webhook`)\n")
	sb.WriteString("3. **Tunnels** - Expose local webhook server via smee.io or cloudflared (`sync-gh tunnel`, `sync-cf tunnel`)\n\n")
	sb.WriteString("**Use cases:**\n")
	sb.WriteString("- Auto-update dependencies when upstream releases\n")
	sb.WriteString("- Trigger rebuilds when CI passes\n")
	sb.WriteString("- Notify on deploy completion\n\n")

	sb.WriteString("## Commands\n\n")

	// Group commands by category
	categories := map[string][]commandInfo{
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
			categories["Core"] = append(categories["Core"], c)
		case "pkg", "binary":
			categories["Package Management"] = append(categories["Package Management"], c)
		case "service", "release":
			categories["Process"] = append(categories["Process"], c)
		case "sync-gh", "sync-cf":
			categories["Sync"] = append(categories["Sync"], c)
		case "docs", "os", "completion":
			categories["Development"] = append(categories["Development"], c)
		default:
			categories["Other"] = append(categories["Other"], c)
		}
	}

	// Category descriptions for context
	categoryDescriptions := map[string]string{
		"Core":               "",
		"Package Management": "",
		"Process":            "",
		"Sync":               "Monitor GitHub and Cloudflare for events (releases, CI, deploys). See [Sync Commands](#sync-commands) above.",
		"Development":        "",
		"Other":              "",
	}

	// Write each category
	categoryOrder := []string{"Core", "Package Management", "Process", "Sync", "Development", "Other"}
	for _, cat := range categoryOrder {
		cmds := categories[cat]
		if len(cmds) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", cat))
		if desc := categoryDescriptions[cat]; desc != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", desc))
		}
		sb.WriteString("| Command | Description |\n")
		sb.WriteString("|---------|-------------|\n")

		for _, c := range cmds {
			sb.WriteString(fmt.Sprintf("| `xplat %s` | %s |\n", c.Name, c.Short))
		}
		sb.WriteString("\n")
	}

	// Detailed command reference
	sb.WriteString("## Command Reference\n\n")
	for _, c := range commands {
		sb.WriteString(fmt.Sprintf("### `xplat %s`\n\n", c.Name))
		sb.WriteString(fmt.Sprintf("%s\n\n", c.Short))

		if len(c.Subcommands) > 0 {
			sb.WriteString("**Subcommands:**\n")
			for _, sub := range c.Subcommands {
				sb.WriteString(fmt.Sprintf("- `%s %s` - %s\n", c.Name, sub.Name, sub.Short))
			}
			sb.WriteString("\n")
		}
	}

	// Write file
	outPath := fmt.Sprintf("%s/README.md", docsOutputDir)
	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		return err
	}

	fmt.Printf("✓ Generated %s\n", outPath)
	return nil
}

func runDocsTaskfile(cmd *cobra.Command, args []string) error {
	root := cmd.Root()
	commands := extractCommands(root)

	var sb strings.Builder

	sb.WriteString("version: '3'\n\n")
	sb.WriteString("# =============================================================================\n")
	sb.WriteString("# xplat - Cross-Platform Taskfile Bootstrapper\n")
	sb.WriteString("# =============================================================================\n")
	sb.WriteString("# Auto-generated from xplat commands. Do not edit directly.\n")
	sb.WriteString("# Regenerate with: xplat docs taskfile\n")
	sb.WriteString("# =============================================================================\n\n")

	sb.WriteString("vars:\n")
	sb.WriteString("  BIN_INSTALL_DIR: '{{if eq OS \"windows\"}}{{.HOME}}/bin{{else}}{{.HOME}}/.local/bin{{end}}'\n")
	sb.WriteString("  XPLAT_BIN: '{{.BIN_INSTALL_DIR}}/xplat{{exeExt}}'\n")
	sb.WriteString("  # Local build binary (for development)\n")
	sb.WriteString("  XPLAT_LOCAL: './xplat{{exeExt}}'\n\n")

	sb.WriteString("tasks:\n")

	// Default task
	sb.WriteString("  default:\n")
	sb.WriteString("    desc: Show available tasks\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - task --list\n\n")

	// Build tasks
	sb.WriteString("  build:\n")
	sb.WriteString("    desc: Build xplat binary\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - go build -o xplat{{exeExt}} .\n\n")

	sb.WriteString("  build:install:\n")
	sb.WriteString("    desc: Build and install xplat to ~/.local/bin\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - mkdir -p \"{{.BIN_INSTALL_DIR}}\"\n")
	sb.WriteString("      - go build -o \"{{.XPLAT_BIN}}\" .\n\n")

	// Test/quality tasks
	sb.WriteString("  test:\n")
	sb.WriteString("    desc: Run tests\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - go test ./... -v\n\n")

	sb.WriteString("  fmt:\n")
	sb.WriteString("    desc: Format code\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - go fmt ./...\n\n")

	sb.WriteString("  lint:\n")
	sb.WriteString("    desc: Lint code\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - go vet ./...\n\n")

	sb.WriteString("  clean:\n")
	sb.WriteString("    desc: Clean build artifacts\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - rm -f xplat xplat.exe\n")
	sb.WriteString("      - rm -rf .task/\n\n")

	// Docs task
	sb.WriteString("  docs:\n")
	sb.WriteString("    desc: Regenerate README.md and Taskfile.yml from code\n")
	sb.WriteString("    cmds:\n")
	sb.WriteString("      - \"{{.XPLAT_LOCAL}} docs all\"\n\n")

	// xplat wrapper tasks (for common operations)
	sb.WriteString("  # ===========================================================================\n")
	sb.WriteString("  # xplat Command Wrappers\n")
	sb.WriteString("  # ===========================================================================\n\n")

	for _, c := range commands {
		if c.Name == "help" || c.Name == "completion" {
			continue
		}

		sb.WriteString(fmt.Sprintf("  xplat:%s:\n", c.Name))
		sb.WriteString(fmt.Sprintf("    desc: \"%s\"\n", c.Short))
		sb.WriteString("    cmds:\n")
		sb.WriteString(fmt.Sprintf("      - \"{{.XPLAT_LOCAL}} %s {{.CLI_ARGS}}\"\n\n", c.Name))
	}

	// Write file
	outPath := fmt.Sprintf("%s/Taskfile.generated.yml", docsOutputDir)
	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		return err
	}

	fmt.Printf("✓ Generated %s\n", outPath)
	return nil
}
