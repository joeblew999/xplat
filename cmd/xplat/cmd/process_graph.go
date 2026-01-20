package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/processcompose"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ProcessGraphCmd generates dependency graphs from process-compose config files.
// Unlike `xplat process graph` which queries a running server, this command
// works offline from the config file - useful for documentation generation.
var ProcessGraphCmd = &cobra.Command{
	Use:   "graph [file]",
	Short: "Generate process dependency graph from config (offline)",
	Long: `Generate a dependency graph from process-compose.yaml config files.

This works OFFLINE - it reads the config file directly, unlike:
  xplat process graph    # Requires running process-compose server

Output formats:
  -f ascii    ASCII tree with dependencies (default)
  -f mermaid  Mermaid flowchart (for docs)
  -f json     JSON structure for tooling
  -f yaml     YAML structure for tooling

Examples:
  xplat process tools graph                    # ASCII from auto-detected config
  xplat process tools graph -f mermaid         # Mermaid for documentation
  xplat process tools graph -f mermaid > deps.md  # Save for docs
  xplat process tools graph pc.yaml            # Specific file
  xplat process tools graph -f json            # JSON for CI/tooling`,
	RunE: runProcessGraph,
}

var processGraphFormat string

func init() {
	ProcessGraphCmd.Flags().StringVarP(&processGraphFormat, "format", "f", "ascii", "Output format: ascii, mermaid, json, yaml")
}

// GraphNode represents a process in the dependency graph.
type GraphNode struct {
	Name      string      `json:"name" yaml:"name"`
	Command   string      `json:"command,omitempty" yaml:"command,omitempty"`
	Disabled  bool        `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Namespace string      `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	DependsOn []string    `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Scheduled bool        `json:"scheduled,omitempty" yaml:"scheduled,omitempty"`
	Schedule  *ScheduleInfo `json:"schedule,omitempty" yaml:"schedule,omitempty"`
}

// ScheduleInfo holds schedule details for display.
type ScheduleInfo struct {
	Type     string `json:"type" yaml:"type"` // "cron" or "interval"
	Value    string `json:"value" yaml:"value"`
	Timezone string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}

// GraphOutput represents the full dependency graph.
type GraphOutput struct {
	File      string      `json:"file" yaml:"file"`
	Version   string      `json:"version" yaml:"version"`
	Processes []GraphNode `json:"processes" yaml:"processes"`
}

func runProcessGraph(cmd *cobra.Command, args []string) error {
	// Find the config file
	var file string
	if len(args) > 0 {
		file = args[0]
	} else {
		// Auto-detect
		for _, name := range config.ProcessComposeSearchOrder() {
			if _, err := os.Stat(name); err == nil {
				file = name
				break
			}
		}
	}

	if file == "" {
		return fmt.Errorf("no process-compose config file found")
	}

	// Parse the config
	pc, err := processcompose.Parse(file)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", file, err)
	}

	// Build graph output
	output := buildGraphOutput(pc, file)

	// Format output
	switch strings.ToLower(processGraphFormat) {
	case "ascii":
		return outputASCII(output)
	case "mermaid":
		return outputMermaid(output)
	case "json":
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "yaml":
		data, err := yaml.Marshal(output)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	default:
		return fmt.Errorf("unknown format: %s (use ascii, mermaid, json, yaml)", processGraphFormat)
	}

	return nil
}

func buildGraphOutput(pc *processcompose.ProcessCompose, file string) GraphOutput {
	output := GraphOutput{
		File:      file,
		Version:   pc.Version,
		Processes: make([]GraphNode, 0, len(pc.Processes)),
	}

	// Convert processes to graph nodes
	for name, proc := range pc.Processes {
		node := GraphNode{
			Name:      name,
			Command:   proc.Command,
			Disabled:  proc.Disabled,
			Namespace: proc.Namespace,
		}

		// Extract dependencies
		for dep := range proc.DependsOn {
			node.DependsOn = append(node.DependsOn, dep)
		}
		sort.Strings(node.DependsOn)

		// Check for schedule
		if proc.Schedule != nil {
			node.Scheduled = true
			node.Schedule = &ScheduleInfo{
				Timezone: proc.Schedule.Timezone,
			}
			if proc.Schedule.Cron != "" {
				node.Schedule.Type = "cron"
				node.Schedule.Value = proc.Schedule.Cron
			} else if proc.Schedule.Interval != "" {
				node.Schedule.Type = "interval"
				node.Schedule.Value = proc.Schedule.Interval
			}
		}

		output.Processes = append(output.Processes, node)
	}

	// Sort by name
	sort.Slice(output.Processes, func(i, j int) bool {
		return output.Processes[i].Name < output.Processes[j].Name
	})

	return output
}

func outputASCII(output GraphOutput) error {
	fmt.Printf("# Process Dependency Graph (%s)\n\n", output.File)

	// Find root processes (no dependencies)
	roots := make([]GraphNode, 0)
	hasParent := make(map[string]bool)

	for _, p := range output.Processes {
		for _, dep := range p.DependsOn {
			hasParent[p.Name] = true
			_ = dep // The dependency exists, p has a parent
		}
	}

	for _, p := range output.Processes {
		if len(p.DependsOn) == 0 {
			roots = append(roots, p)
		}
	}

	// Build process lookup
	byName := make(map[string]GraphNode)
	for _, p := range output.Processes {
		byName[p.Name] = p
	}

	// Print tree starting from roots
	if len(roots) == 0 {
		// No clear roots, just list all
		for _, p := range output.Processes {
			printProcessASCII(p, "", true)
		}
	} else {
		for i, root := range roots {
			isLast := i == len(roots)-1
			printTreeASCII(root, byName, "", isLast, make(map[string]bool))
		}
	}

	// Summary
	fmt.Printf("\n%d processes", len(output.Processes))
	scheduledCount := 0
	disabledCount := 0
	for _, p := range output.Processes {
		if p.Scheduled {
			scheduledCount++
		}
		if p.Disabled {
			disabledCount++
		}
	}
	if scheduledCount > 0 {
		fmt.Printf(", %d scheduled", scheduledCount)
	}
	if disabledCount > 0 {
		fmt.Printf(", %d disabled", disabledCount)
	}
	fmt.Println()

	return nil
}

func printProcessASCII(p GraphNode, prefix string, isLast bool) {
	marker := "├── "
	if isLast {
		marker = "└── "
	}

	status := ""
	if p.Disabled {
		status = " [disabled]"
	}
	if p.Scheduled {
		sched := ""
		if p.Schedule != nil {
			sched = fmt.Sprintf(" (%s: %s)", p.Schedule.Type, p.Schedule.Value)
		}
		status += " [scheduled" + sched + "]"
	}

	fmt.Printf("%s%s%s%s\n", prefix, marker, p.Name, status)
}

func printTreeASCII(p GraphNode, byName map[string]GraphNode, prefix string, isLast bool, visited map[string]bool) {
	// Prevent infinite loops
	if visited[p.Name] {
		return
	}
	visited[p.Name] = true

	printProcessASCII(p, prefix, isLast)

	// Find processes that depend on this one
	var children []GraphNode
	for _, proc := range byName {
		for _, dep := range proc.DependsOn {
			if dep == p.Name {
				children = append(children, proc)
			}
		}
	}

	// Sort children by name
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})

	// Print children
	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	for i, child := range children {
		childIsLast := i == len(children)-1
		printTreeASCII(child, byName, childPrefix, childIsLast, visited)
	}
}

func outputMermaid(output GraphOutput) error {
	fmt.Println("```mermaid")
	fmt.Println("flowchart TD")

	// Define nodes with styling
	for _, p := range output.Processes {
		label := p.Name
		style := ""

		if p.Disabled {
			style = ":::disabled"
			label += "<br/>[disabled]"
		} else if p.Scheduled {
			style = ":::scheduled"
			if p.Schedule != nil {
				label += fmt.Sprintf("<br/>[%s: %s]", p.Schedule.Type, p.Schedule.Value)
			}
		}

		fmt.Printf("    %s[\"%s\"]%s\n", sanitizeMermaidID(p.Name), label, style)
	}

	fmt.Println()

	// Define edges (dependencies)
	for _, p := range output.Processes {
		for _, dep := range p.DependsOn {
			// Arrow from dependency to process (dep must be ready before p starts)
			fmt.Printf("    %s --> %s\n", sanitizeMermaidID(dep), sanitizeMermaidID(p.Name))
		}
	}

	// Add styling
	fmt.Println()
	fmt.Println("    classDef disabled fill:#gray,stroke:#666,stroke-dasharray: 5 5")
	fmt.Println("    classDef scheduled fill:#e1f5fe,stroke:#0288d1")

	fmt.Println("```")
	return nil
}

// sanitizeMermaidID replaces characters that are invalid in Mermaid IDs.
func sanitizeMermaidID(name string) string {
	// Replace hyphens and dots with underscores
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return name
}
