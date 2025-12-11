package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/joeblew999/xplat/internal/taskfile"
	"github.com/spf13/cobra"
)

// ArchetypeCmd is the parent command for archetype operations
var ArchetypeCmd = &cobra.Command{
	Use:   "archetype",
	Short: "Taskfile archetype operations",
	Long: `Manage and inspect Taskfile archetypes.

Archetypes define the purpose and required structure of Taskfiles:
  - tool:        Binary we build and release (has _BIN, _VERSION, _REPO, _CGO)
  - external:    External binary we install (has _BIN, _VERSION, no _REPO)
  - builder:     Provides build infrastructure (has _BUILD_* vars)
  - aggregation: Groups children via includes: section
  - bootstrap:   Self-bootstrapping tool (xplat itself)

Use 'xplat archetype list' to see all archetypes and their requirements.
Use 'xplat archetype detect <file>' to identify a file's archetype.`,
}

var archetypeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all archetypes with their requirements",
	RunE:  runArchetypeList,
}

var archetypeDetectCmd = &cobra.Command{
	Use:   "detect <file|dir>",
	Short: "Detect archetype for a Taskfile or directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runArchetypeDetect,
}

var archetypeExplainCmd = &cobra.Command{
	Use:   "explain <archetype>",
	Short: "Explain a specific archetype in detail",
	Args:  cobra.ExactArgs(1),
	RunE:  runArchetypeExplain,
}

func init() {
	ArchetypeCmd.AddCommand(archetypeListCmd)
	ArchetypeCmd.AddCommand(archetypeDetectCmd)
	ArchetypeCmd.AddCommand(archetypeExplainCmd)
}

func runArchetypeList(cmd *cobra.Command, args []string) error {
	fmt.Println("Taskfile Archetypes")
	fmt.Println("===================")
	fmt.Println()

	// Tool
	fmt.Println("TOOL")
	fmt.Println("  Purpose:  Binary we build from source and release to GitHub")
	fmt.Println("  Examples: dummy.yml, analytics.yml, sitecheck.yml, genlogo.yml")
	fmt.Println("  Detection: Has _BIN + _VERSION + _REPO vars")
	fmt.Println()
	fmt.Println("  Required Vars:")
	fmt.Println("    *_VERSION  - Version string (from xplat.env)")
	fmt.Println("    *_REPO     - GitHub owner/repo")
	fmt.Println("    *_BIN      - Binary name with {{exeExt}}")
	fmt.Println("    *_CGO      - '0' or '1' (cross-compile capability)")
	fmt.Println()
	fmt.Println("  Required Tasks:")
	fmt.Println("    check:deps     - Ensure binary available (with status:)")
	fmt.Println("    release:build  - Build for release")
	fmt.Println("    release:test   - Test built binary")
	fmt.Println()

	// External
	fmt.Println("EXTERNAL")
	fmt.Println("  Purpose:  External binary we install (not built by us)")
	fmt.Println("  Examples: pc.yml (process-compose), tui.yml (task-ui)")
	fmt.Println("  Detection: Has _BIN + _VERSION but NO _REPO")
	fmt.Println()
	fmt.Println("  Required Vars:")
	fmt.Println("    *_VERSION  - Version string")
	fmt.Println("    *_BIN      - Binary name with {{exeExt}}")
	fmt.Println()
	fmt.Println("  Required Tasks:")
	fmt.Println("    check:deps  - Ensure binary available (with status:)")
	fmt.Println()

	// Builder
	fmt.Println("BUILDER")
	fmt.Println("  Purpose:  Provides build infrastructure for Tools")
	fmt.Println("  Examples: golang.yml, bun.yml")
	fmt.Println("  Detection: Has *_BUILD_* vars (e.g., GO_BUILD_DIR)")
	fmt.Println()
	fmt.Println("  Required Vars:")
	fmt.Println("    *_BUILD_*  - Build configuration vars")
	fmt.Println()
	fmt.Println("  Required Tasks:")
	fmt.Println("    check:deps  - Ensure toolchain available")
	fmt.Println("    build       - Build a binary (accepts BIN, VERSION, SOURCE, CGO)")
	fmt.Println()

	// Aggregation
	fmt.Println("AGGREGATION")
	fmt.Println("  Purpose:  Groups related taskfiles, provides unified lifecycle")
	fmt.Println("  Examples: toolchain.yml, tools.yml")
	fmt.Println("  Detection: Has includes: section")
	fmt.Println()
	fmt.Println("  No required vars - delegates to children via includes:")
	fmt.Println()

	// Bootstrap
	fmt.Println("BOOTSTRAP")
	fmt.Println("  Purpose:  Self-bootstrapping tool (xplat itself)")
	fmt.Println("  Examples: Taskfile.xplat.yml")
	fmt.Println("  Detection: Has XPLAT_VERSION + XPLAT_REPO + XPLAT_BIN")
	fmt.Println()
	fmt.Println("  Required Vars:")
	fmt.Println("    XPLAT_VERSION  - Version string")
	fmt.Println("    XPLAT_REPO     - GitHub owner/repo")
	fmt.Println("    XPLAT_BIN      - Binary name with {{exeExt}}")
	fmt.Println()
	fmt.Println("  Required Tasks:")
	fmt.Println("    check:deps  - Self-bootstrap (build from source or download)")
	fmt.Println()
	fmt.Println("  Special: Cannot use xplat binary:install (chicken-egg problem)")
	fmt.Println()

	// Unknown
	fmt.Println("UNKNOWN")
	fmt.Println("  Purpose:  Project workflow taskfile with no strict archetype")
	fmt.Println("  Examples: hugo.yml, translate.yml")
	fmt.Println("  Detection: Does not match other archetypes")
	fmt.Println()
	fmt.Println("  No strict requirements - these are project-specific workflows")

	return nil
}

func runArchetypeDetect(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access %s: %w", path, err)
	}

	var files []string
	if info.IsDir() {
		files, err = taskfile.FindTaskfiles(path)
		if err != nil {
			return err
		}
	} else {
		files = []string{path}
	}

	if len(files) == 0 {
		fmt.Println("No Taskfiles found")
		return nil
	}

	// Group by archetype
	byArchetype := make(map[taskfile.Archetype][]string)

	for _, f := range files {
		tf, err := taskfile.Parse(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v\n", f, err)
			continue
		}

		arch := taskfile.DetectArchetype(tf)
		relPath, _ := filepath.Rel(".", f)
		if relPath == "" {
			relPath = f
		}
		byArchetype[arch.Type] = append(byArchetype[arch.Type], relPath)
	}

	// Print summary
	fmt.Printf("Archetype Detection (%d files)\n", len(files))
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println()

	// Print in alphabetical order by archetype name
	order := []taskfile.Archetype{
		taskfile.ArchetypeAggregation,
		taskfile.ArchetypeBootstrap,
		taskfile.ArchetypeBuilder,
		taskfile.ArchetypeExternal,
		taskfile.ArchetypeTool,
		taskfile.ArchetypeUnknown,
	}

	for _, arch := range order {
		files := byArchetype[arch]
		if len(files) == 0 {
			continue
		}

		// Sort files alphabetically
		sort.Strings(files)

		fmt.Printf("%s (%d)\n", strings.ToUpper(string(arch)), len(files))
		for _, f := range files {
			fmt.Printf("  %s\n", f)
		}
		fmt.Println()
	}

	return nil
}

func runArchetypeExplain(cmd *cobra.Command, args []string) error {
	archetype := strings.ToLower(args[0])

	switch archetype {
	case "tool":
		explainTool()
	case "external":
		explainExternal()
	case "builder":
		explainBuilder()
	case "aggregation":
		explainAggregation()
	case "bootstrap":
		explainBootstrap()
	case "unknown":
		explainUnknown()
	default:
		return fmt.Errorf("unknown archetype: %s\nValid archetypes: tool, external, builder, aggregation, bootstrap, unknown", archetype)
	}

	return nil
}

func explainTool() {
	fmt.Println(`TOOL ARCHETYPE
==============

Purpose: Binary we build from source and release to GitHub.

These are Go binaries in cmd/<name>/ that we:
1. Build from source during development
2. Release as GitHub releases for distribution
3. Install via 'xplat binary install'

REQUIRED VARIABLES
------------------
Variable naming follows the pattern <TOOLNAME>_*:

  *_VERSION   Version string, typically from xplat.env
              Example: DUMMY_VERSION: '{{.DUMMY_VERSION}}'

  *_REPO      GitHub owner/repo for releases
              Example: DUMMY_REPO: joeblew999/ubuntu-website

  *_BIN       Binary name with {{exeExt}} for Windows
              Example: DUMMY_BIN: 'dummy{{exeExt}}'

  *_CGO       CGO requirement: '0' (pure Go) or '1' (needs C)
              Example: DUMMY_CGO: '0'

REQUIRED TASKS
--------------
  check:deps      Ensure binary is available
                  MUST have status: section for idempotency

  release:build   Build binary for release
                  Calls :toolchain:golang:build with vars

  release:test    Test the built release binary
                  Calls :toolchain:golang:build:test

EXAMPLE
-------
vars:
  DUMMY_VERSION: '{{.DUMMY_VERSION}}'
  DUMMY_REPO: joeblew999/ubuntu-website
  DUMMY_BIN: 'dummy{{exeExt}}'
  DUMMY_CGO: '0'

tasks:
  check:deps:
    status:
      - test -f "{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}"
    cmds:
      - task: :tools:xplat:binary:install
        vars:
          CLI_ARGS: 'dummy {{.DUMMY_VERSION}} {{.DUMMY_REPO}}'

  release:build:
    cmds:
      - task: :toolchain:golang:build
        vars:
          BIN: '{{.DUMMY_BIN}}'
          VERSION: '{{.DUMMY_VERSION}}'
          SOURCE: '{{.ROOT_DIR}}/cmd/dummy'
          CGO: '{{.DUMMY_CGO}}'

  release:test:
    cmds:
      - task: :toolchain:golang:build:test
        vars:
          BIN: '{{.DUMMY_BIN}}'`)
}

func explainExternal() {
	fmt.Println(`EXTERNAL ARCHETYPE
==================

Purpose: External binary we install but don't build.

These are third-party tools that we download and install,
not binaries we build ourselves.

REQUIRED VARIABLES
------------------
  *_VERSION   Version to install
              Example: PC_VERSION: v1.0.0

  *_BIN       Binary name with {{exeExt}}
              Example: PC_BIN: 'process-compose{{exeExt}}'

NO _REPO or _CGO - different installation mechanism.

REQUIRED TASKS
--------------
  check:deps    Ensure binary is available
                MUST have status: section for idempotency
                Uses custom install logic (curl, brew, etc.)

EXAMPLE
-------
vars:
  PC_VERSION: v1.0.0
  PC_BIN: 'process-compose{{exeExt}}'

tasks:
  check:deps:
    status:
      - command -v process-compose
    cmds:
      - |
        # Custom install logic
        curl -L https://... -o {{.BIN_INSTALL_DIR}}/{{.PC_BIN}}`)
}

func explainBuilder() {
	fmt.Println(`BUILDER ARCHETYPE
=================

Purpose: Provides build infrastructure for Tool taskfiles.

Builders are toolchain providers - they define HOW to build,
while Tools define WHAT to build.

REQUIRED VARIABLES
------------------
Pattern: <LANG>_BUILD_*

  *_BUILD_DIR        Output directory for builds
  *_BUILD_PLATFORMS  Target platforms (optional)
  *_BUILD_LDFLAGS    Linker flags (optional)

REQUIRED TASKS
--------------
  check:deps    Ensure toolchain is available
                Example: go version check

  build         Build a binary
                Accepts: BIN, VERSION, SOURCE, CGO
                Called by Tool's release:build

EXAMPLE
-------
vars:
  GO_BUILD_DIR: '{{.ROOT_DIR}}/bin'
  GO_BUILD_LDFLAGS: '-s -w'

tasks:
  check:deps:
    cmds:
      - go version

  build:
    requires:
      vars: [BIN, VERSION, SOURCE]
    cmds:
      - go build -ldflags "{{.GO_BUILD_LDFLAGS}}" -o {{.GO_BUILD_DIR}}/{{.BIN}} {{.SOURCE}}`)
}

func explainAggregation() {
	fmt.Println(`AGGREGATION ARCHETYPE
=====================

Purpose: Groups related taskfiles and provides unified lifecycle.

Aggregation taskfiles use the includes: section to compose
multiple child taskfiles into a namespace.

DETECTION
---------
Has includes: section (non-empty)

NO REQUIRED VARIABLES
---------------------
Aggregation files delegate to children - they don't define
their own vars.

STRUCTURE
---------
includes:
  golang: ./Taskfile.golang.yml
  bun: ./Taskfile.bun.yml

This creates namespaced tasks:
  task golang:build
  task bun:build

EXAMPLE
-------
version: '3'

includes:
  golang:
    taskfile: ./Taskfile.golang.yml
    dir: '{{.ROOT_DIR}}'
  bun:
    taskfile: ./Taskfile.bun.yml
    dir: '{{.ROOT_DIR}}'

tasks:
  check:deps:
    desc: Check all toolchain dependencies
    deps:
      - golang:check:deps
      - bun:check:deps`)
}

func explainBootstrap() {
	fmt.Println(`BOOTSTRAP ARCHETYPE
===================

Purpose: Self-bootstrapping tool that must install itself.

The Bootstrap archetype is special - it's for xplat itself.
xplat cannot use 'xplat binary:install' to install xplat
(chicken-egg problem).

REQUIRED VARIABLES
------------------
  XPLAT_VERSION   Version string
                  Example: XPLAT_VERSION: v0.2.0

  XPLAT_REPO      GitHub owner/repo
                  Example: XPLAT_REPO: joeblew999/ubuntu-website

  XPLAT_BIN       Binary name with {{exeExt}}
                  Example: XPLAT_BIN: 'xplat{{exeExt}}'

REQUIRED TASKS
--------------
  check:deps    Self-bootstrap installation
                Strategy 1: Build from source if Go available
                Strategy 2: Download from GitHub Release

SPECIAL CHARACTERISTICS
-----------------------
1. Cannot depend on xplat binary (it IS xplat)
2. Must work in fresh environments (no tools pre-installed)
3. Provides wrapper tasks for other tools to use:
   - binary:install
   - release:build
   - release:list

WHY BOOTSTRAP IS DIFFERENT FROM TOOL
-------------------------------------
Tool archetype uses xplat binary:install for installation.
Bootstrap IS the xplat binary, so it must bootstrap itself.

Tool requires: check:deps, release:build, release:test
Bootstrap requires: only check:deps (self-contained)`)
}

func explainUnknown() {
	fmt.Println(`UNKNOWN ARCHETYPE
=================

Purpose: Project workflow taskfile with no strict archetype.

These are legitimate taskfiles that don't fit the other archetypes.
They handle project-specific workflows like:

  - hugo.yml     - Hugo site building
  - translate.yml - Translation management
  - dev.yml      - Development workflows

NO STRICT REQUIREMENTS
----------------------
Unknown archetype files are not validated for specific vars
or tasks. They have their own lifecycle and conventions.

WHEN IS THIS OK?
----------------
- Project-specific workflows (not reusable tools)
- External tool usage (Hugo, npm, etc.)
- One-off automation tasks

WHEN SHOULD YOU CHANGE ARCHETYPE?
---------------------------------
If a taskfile:
- Builds a binary -> should be Tool
- Installs an external binary -> should be External
- Provides build infrastructure -> should be Builder
- Groups child taskfiles -> should be Aggregation`)
}
