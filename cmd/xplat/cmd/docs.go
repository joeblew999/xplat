// Package cmd provides CLI commands for xplat.
//
// docs.go - Generate documentation from xplat commands
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/joeblew999/xplat/internal/templates"
	"github.com/spf13/cobra"
)

// DocsCmd generates documentation from xplat commands.
// This is an INTERNAL command for xplat developers only.
// Users of plat-* repos should use 'xplat gen' instead.
var DocsCmd = &cobra.Command{
	Use:   "internal:docs",
	Short: "Generate xplat's own documentation (for xplat developers)",
	Long: `Generate xplat's own README.md and Taskfile.yml from its command structure.

This is for xplat developers only - NOT for plat-* repos.
If you're working in a plat-* repo, use 'xplat gen all' instead.

Examples:
  xplat internal:docs all         # Generate README.md + Taskfile.yml
  xplat internal:docs readme      # Generate README.md only
  xplat internal:docs taskfile    # Generate Taskfile.yml only`,
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
	Short: "Generate all documentation (README.md + Taskfile.yml)",
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
