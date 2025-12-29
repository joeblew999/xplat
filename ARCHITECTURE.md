# xplat Architecture

## Directory Structure

```
xplat/
├── cmd/xplat/
│   ├── main.go           # Entry point, registers all commands
│   └── cmd/              # CLI commands (thin wrappers)
│       ├── manifest.go   # xplat manifest * commands
│       ├── taskfile.go   # xplat taskfile * commands
│       ├── binary.go     # xplat binary * commands
│       └── ...           # Other command files
│
├── internal/             # Business logic (not exported)
│   ├── manifest/         # Manifest operations
│   │   ├── types.go      # Manifest struct and related types
│   │   ├── loader.go     # Load/parse manifests from file/URL/GitHub
│   │   ├── generate.go   # Generate .env, process-compose, Taskfile
│   │   ├── install.go    # Binary installation logic
│   │   ├── check.go      # Deep validation against filesystem
│   │   └── init.go       # Scaffold new manifests
│   │
│   ├── taskfile/         # Taskfile operations
│   │   └── archetype.go  # Detect Taskfile archetypes
│   │
│   └── ...               # Other internal packages
│
└── pkg/                  # Exported library packages (if any)
```

## Design Principles

### 1. cmd/ is Thin Wrappers Only

The `cmd/xplat/cmd/` directory contains only:
- Cobra command definitions
- Flag parsing
- Calling internal functions
- Printing output to user

Business logic belongs in `internal/`.

**Good:**
```go
func runManifestCheck(cmd *cobra.Command, args []string) error {
    result := manifest.Check(m, repoPath)  // Call internal
    fmt.Printf("✓ %s\n", result.Name)      // Print to user
    return nil
}
```

**Bad:**
```go
func runManifestCheck(cmd *cobra.Command, args []string) error {
    // Don't put 200 lines of validation logic here!
    // Move it to internal/manifest/check.go
}
```

### 2. internal/ Contains Business Logic

Each package in `internal/` is self-contained:
- `internal/manifest/` - Everything about xplat.yaml manifests
- `internal/taskfile/` - Everything about Taskfile validation
- `internal/binary/` - Binary installation helpers

### 3. Naming Convention

| File | Purpose |
|------|---------|
| `types.go` | Struct definitions and type methods |
| `loader.go` | Loading/parsing from various sources |
| `generate.go` | Generating output files |
| `check.go` | Validation logic |
| `init.go` | Scaffolding/initialization |
| `install.go` | Installation logic |

## Package Responsibilities

### internal/manifest

Handles xplat.yaml package manifests:

| Function | Description |
|----------|-------------|
| `NewLoader()` | Create a manifest loader |
| `loader.LoadFile()` | Load from local file |
| `loader.LoadDir()` | Load from directory |
| `loader.LoadGitHub()` | Load from GitHub repo |
| `loader.DiscoverPlat()` | Find all plat-* manifests |
| `NewGenerator()` | Create file generator |
| `gen.GenerateEnvExample()` | Generate .env.example |
| `gen.GenerateProcessCompose()` | Generate process-compose.yaml |
| `gen.GenerateTaskfile()` | Generate Taskfile.yml |
| `Check()` | Validate manifest against filesystem |
| `CheckAll()` | Validate all plat-* manifests |
| `Init()` | Scaffold new manifest |

### internal/taskfile

Handles Taskfile validation and archetypes:

| Function | Description |
|----------|-------------|
| `Parse()` | Parse a Taskfile |
| `DetectArchetype()` | Detect Taskfile archetype |
| `FindTaskfiles()` | Find Taskfiles in directory |

## Future: Service Mode

xplat will eventually run as a long-running service for:
- Health monitoring of processes
- Scheduled task execution
- File watching and hot reload
- Centralized logging

The service architecture will be:

```
internal/
├── service/
│   ├── server.go     # HTTP/gRPC server
│   ├── scheduler.go  # Task scheduling
│   ├── watcher.go    # File watching
│   └── health.go     # Health checks
```

The service will reuse all existing internal packages.
