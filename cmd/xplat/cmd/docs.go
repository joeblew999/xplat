// Package cmd provides CLI commands for xplat.
//
// docs.go - Generate documentation from xplat commands
package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/joeblew999/xplat/internal/process"
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

var docsProcessCmd = &cobra.Command{
	Use:   "process",
	Short: "Generate process-compose.generated.yaml from registry",
	RunE:  runDocsProcess,
}

var docsAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Generate all documentation",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := runDocsReadme(cmd, args); err != nil {
			return err
		}
		if err := runDocsTaskfile(cmd, args); err != nil {
			return err
		}
		return runDocsProcess(cmd, args)
	},
}

var docsOutputDir string

func init() {
	DocsCmd.PersistentFlags().StringVarP(&docsOutputDir, "output", "o", ".", "Output directory")

	DocsCmd.AddCommand(docsReadmeCmd)
	DocsCmd.AddCommand(docsTaskfileCmd)
	DocsCmd.AddCommand(docsProcessCmd)
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
	sb.WriteString("Cross-platform Taskfile bootstrapper - a single binary that embeds:\n")
	sb.WriteString("- **Task** (taskfile runner)\n")
	sb.WriteString("- **Process-Compose** (process orchestration)\n")
	sb.WriteString("- **Cross-platform utilities** (rm, cp, mv, glob, etc.)\n\n")

	sb.WriteString("## Installation\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("# Build from source\n")
	sb.WriteString("go build -o xplat ./cmd/xplat/\n\n")
	sb.WriteString("# Install to ~/.local/bin\n")
	sb.WriteString("task build:install\n")
	sb.WriteString("```\n\n")

	sb.WriteString("## Commands\n\n")

	// Group commands by category
	categories := map[string][]commandInfo{
		"Core":              {},
		"File Operations":   {},
		"Utilities":         {},
		"Package Management": {},
		"Taskfile":          {},
		"Process":           {},
		"Other":             {},
	}

	for _, c := range commands {
		switch c.Name {
		case "task", "run", "which", "version":
			categories["Core"] = append(categories["Core"], c)
		case "rm", "mkdir", "cp", "mv", "cat", "touch":
			categories["File Operations"] = append(categories["File Operations"], c)
		case "glob", "env", "jq", "extract", "fetch":
			categories["Utilities"] = append(categories["Utilities"], c)
		case "pkg", "binary":
			categories["Package Management"] = append(categories["Package Management"], c)
		case "fmt", "lint", "archetype", "test":
			categories["Taskfile"] = append(categories["Taskfile"], c)
		case "process", "process-gen", "dev":
			categories["Process"] = append(categories["Process"], c)
		default:
			categories["Other"] = append(categories["Other"], c)
		}
	}

	// Write each category
	categoryOrder := []string{"Core", "File Operations", "Utilities", "Package Management", "Taskfile", "Process", "Other"}
	for _, cat := range categoryOrder {
		cmds := categories[cat]
		if len(cmds) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", cat))
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
	sb.WriteString("      - '{{.XPLAT_LOCAL}}' docs all\n\n")

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
		sb.WriteString(fmt.Sprintf("      - '{{.XPLAT_LOCAL}}' %s {{.CLI_ARGS}}\n\n", c.Name))
	}

	// Write file
	outPath := fmt.Sprintf("%s/Taskfile.generated.yml", docsOutputDir)
	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		return err
	}

	fmt.Printf("✓ Generated %s\n", outPath)
	return nil
}

func runDocsProcess(cmd *cobra.Command, args []string) error {
	// Use the process-gen generate command logic
	gen := process.NewGenerator(fmt.Sprintf("%s/process-compose.generated.yaml", docsOutputDir))

	config, err := gen.GenerateFromRegistry()
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	if len(config.Processes) == 0 {
		fmt.Println("No packages with process configurations found in registry.")
		return nil
	}

	if err := gen.Write(config); err != nil {
		return err
	}

	fmt.Printf("✓ Generated %s with %d processes\n", gen.ConfigPath(), len(config.Processes))
	return nil
}
