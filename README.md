# xplat

**One binary to bootstrap and run any plat-* project.**

## Why?

Instead of installing Task, process-compose, and various CLIs separately,
xplat embeds them all. One binary, works on macOS/Linux/Windows.

## ✨ Composability - The Key Feature

**Reuse tasks and processes across projects.**

xplat enables composability: install packages from other xplat projects
and immediately use their tasks and processes in your project.

```bash
# Install a package
xplat pkg install plat-nats --with-process

# Generate includes from installed packages
xplat gen taskfile    # Creates Taskfile.generated.yml with remote includes
xplat gen process     # Creates pc.generated.yaml with processes

# Now use tasks from the installed package!
task nats:run         # Run tasks defined in plat-nats
process-compose up    # Run processes including plat-nats
```

**How it works:**
1. `xplat pkg install` downloads binaries and records package info in `xplat-lock.yaml`
2. `xplat gen taskfile` reads the lockfile and generates remote Taskfile includes
3. `xplat gen process` reads the lockfile and generates process-compose definitions

This lets you build a platform from composable pieces - each `plat-*` project
can expose tasks and processes that other projects can reuse.

## Quick Start

```bash
# 1. Bootstrap a new project
xplat manifest bootstrap

# 2. Generate project files from xplat.yaml
xplat gen all

# 3. Build/test/lint (embedded Task)
xplat task build
xplat task test

# 4. Run services (embedded process-compose)
xplat process

# 5. Install packages from registry
xplat pkg install <name>
```

## Installation

```bash
# Build from source
go build -o xplat .

# Or install to ~/.local/bin
go build -o ~/.local/bin/xplat .
```

## Architecture

xplat solves the problem of consistent tooling across multiple `plat-*` projects on Mac/Linux/Windows.

| Component | Purpose |
|-----------|--------|
| **Embedded Task** | Declarative build system. Taskfile.yml defines build/test/lint. |
| **Embedded process-compose** | Multi-process orchestration. Run app + dependencies together. |
| **xplat.yaml manifest** | Single source of truth: language, binary, env vars, processes. |
| **gen commands** | Generate CI, .gitignore, .env from manifest. Change manifest, regenerate. |
| **pkg registry** | Shared tooling. Install a package = binary + taskfile + process config. |
| **os utilities** | Cross-platform primitives (rm, cp, glob) that behave identically everywhere. |
| **sync-gh / sync-cf** | Watch external services (GitHub, Cloudflare) for events. No vendor CLI needed. |

**The pattern:**
```
xplat.yaml (manifest) → gen → Taskfile.yml, process-compose.yaml, CI workflow
                       ↓
                    xplat task build    (runs tasks)
                    xplat process       (runs services)
```

## Sync Commands

The `sync-gh` and `sync-cf` commands monitor external services without requiring vendor CLIs.

**Why?** You often need to react to external events:
- A dependency released a new version (GitHub release)
- CI workflow completed (GitHub Actions)
- A deploy finished (Cloudflare Pages)

**How it works:**
1. **Polling** - Periodically check APIs for changes (`sync-gh poll`, `sync-cf poll`)
2. **Webhooks** - Receive push notifications from services (`sync-gh webhook`, `sync-cf webhook`)
3. **Tunnels** - Expose local webhook server via Cloudflare tunnel (`sync-cf tunnel`)

**Use cases:**
- Auto-update dependencies when upstream releases
- Trigger rebuilds when CI passes
- Notify on deploy completion

## Commands

### Core

| Command | Description |
|---------|-------------|
| `xplat gen` | Generate project files from YOUR local xplat.yaml |
| `xplat manifest` | Inspect, validate, and bootstrap xplat.yaml manifests |
| `xplat process` | Process orchestration (embedded process-compose) |
| `xplat run` | Run a managed tool |
| `xplat task` | Run Taskfile tasks (embedded Task runner) |
| `xplat update` | Update xplat to the latest version |
| `xplat version` | Print xplat version |

### Package Management

| Command | Description |
|---------|-------------|
| `xplat binary` | Binary management commands |
| `xplat pkg` | Install packages from REMOTE registry (binaries, taskfiles, processes) |

### Process

| Command | Description |
|---------|-------------|
| `xplat release` | Release build orchestration |
| `xplat service` | Manage xplat as a system service |

### Sync

Monitor GitHub and Cloudflare for events (releases, CI, deploys). See [Sync Commands](#sync-commands) above.

| Command | Description |
|---------|-------------|
| `xplat sync-cf` | Cloudflare sync operations (no wrangler CLI required) |
| `xplat sync-gh` | GitHub sync operations (no gh CLI required) |

### Development

| Command | Description |
|---------|-------------|
| `xplat completion` | Generate the autocompletion script for the specified shell |
| `xplat os` | Cross-platform OS utilities |

### Other

| Command | Description |
|---------|-------------|
| `xplat help` | Help about any command |
| `xplat internal:docs` | Generate xplat's own documentation (for xplat developers) |
| `xplat mcp` | MCP (Model Context Protocol) server |
| `xplat ui` | Start Task UI web interface |

## Command Reference

### `xplat binary`

Binary management commands

**Subcommands:**
- `binary install` - Install a binary (build from source or download)

### `xplat completion`

Generate the autocompletion script for the specified shell

**Subcommands:**
- `completion bash` - Generate the autocompletion script for bash
- `completion fish` - Generate the autocompletion script for fish
- `completion powershell` - Generate the autocompletion script for powershell
- `completion zsh` - Generate the autocompletion script for zsh

### `xplat gen`

Generate project files from YOUR local xplat.yaml

**Subcommands:**
- `gen all` - Generate all files from manifest
- `gen env` - Generate .env.example
- `gen gitignore` - Generate .gitignore
- `gen process` - Generate pc.generated.yaml with processes from installed packages
- `gen taskfile` - Generate Taskfile.generated.yml with remote includes from installed packages
- `gen workflow` - Generate .github/workflows/ci.yml

### `xplat help`

Help about any command

### `xplat internal:docs`

Generate xplat's own documentation (for xplat developers)

**Subcommands:**
- `internal:docs all` - Generate all documentation (README.md + Taskfile.yml)
- `internal:docs readme` - Generate README.md from xplat commands
- `internal:docs taskfile` - Generate Taskfile.yml command wrappers

### `xplat manifest`

Inspect, validate, and bootstrap xplat.yaml manifests

**Subcommands:**
- `manifest bootstrap` - Bootstrap a plat-* repository with standard files
- `manifest check` - Deep validation of manifest against filesystem
- `manifest discover` - Discover manifests in plat-* directories
- `manifest discover-github` - Discover manifests from GitHub plat-* repos
- `manifest init` - Initialize a new xplat.yaml manifest
- `manifest install` - Install binary from manifest
- `manifest install-all` - Install binaries from all discovered manifests
- `manifest show` - Show manifest details
- `manifest validate` - Validate an xplat.yaml manifest

### `xplat mcp`

MCP (Model Context Protocol) server

**Subcommands:**
- `mcp config` - Show MCP configuration for AI IDEs
- `mcp list` - List tasks that would be exposed as MCP tools
- `mcp serve` - Start MCP server (stdio or HTTP transport)

### `xplat os`

Cross-platform OS utilities

**Subcommands:**
- `os cat` - Print file contents
- `os cp` - Copy files or directories
- `os env` - Get environment variable
- `os envsubst` - Substitute environment variables in text
- `os extract` - Extract archives (zip, tar.gz, tar.bz2, tar.xz, 7z, rar)
- `os fetch` - Download files with optional archive extraction
- `os git` - Git operations (no git binary required)
- `os glob` - Expand glob pattern
- `os jq` - Process JSON with jq syntax
- `os mkdir` - Create directories
- `os mv` - Move or rename files and directories
- `os rm` - Remove files or directories
- `os touch` - Create files or update timestamps
- `os version-file` - Read or write .version file
- `os which` - Find binary in managed locations or PATH

### `xplat pkg`

Install packages from REMOTE registry (binaries, taskfiles, processes)

**Subcommands:**
- `pkg add-process` - Add a package's process to process-compose.yaml
- `pkg info` - Show package details
- `pkg install` - Install a package (binary + taskfile)
- `pkg list` - List available packages
- `pkg list-processes` - List packages with process configurations
- `pkg remove` - Remove a package (binary + taskfile include)
- `pkg remove-process` - Remove a package's process from process-compose.yaml

### `xplat process`

Process orchestration (embedded process-compose)

**Subcommands:**
- `process tools` - Process-compose validation and formatting tools

### `xplat release`

Release build orchestration

**Subcommands:**
- `release binary-name` - Print binary filename for current platform
- `release build` - Build a tool for release
- `release list` - List built release binaries for a tool
- `release matrix` - Output platform build matrix for a tool

### `xplat run`

Run a managed tool

### `xplat service`

Manage xplat as a system service

**Subcommands:**
- `service config` - Configure service settings
- `service install` - Add current project to registry and install OS service
- `service list` - List all registered projects
- `service restart` - Restart the xplat service
- `service start` - Start the xplat service
- `service status` - Check service status
- `service stop` - Stop the xplat service
- `service uninstall` - Remove current project from registry

### `xplat sync-cf`

Cloudflare sync operations (no wrangler CLI required)

**Subcommands:**
- `sync-cf auth` - Set up Cloudflare credentials interactively
- `sync-cf check` - Check if cloudflared is installed
- `sync-cf install` - Install cloudflared
- `sync-cf poll` - Poll CF audit logs continuously
- `sync-cf receive` - Receive events from CF Worker (round-trip validation)
- `sync-cf receive-state` - Show current receive state (processed events)
- `sync-cf tunnel` - Start cloudflared tunnel (quick or named)
- `sync-cf tunnel-create` - Create a new named tunnel
- `sync-cf tunnel-delete` - Delete a named tunnel
- `sync-cf tunnel-list` - List existing named tunnels
- `sync-cf tunnel-login` - Authenticate cloudflared with Cloudflare
- `sync-cf tunnel-route` - Add DNS route for a tunnel
- `sync-cf webhook` - Start CF webhook server
- `sync-cf worker` - Manage sync-cf Cloudflare Worker

### `xplat sync-gh`

GitHub sync operations (no gh CLI required)

**Subcommands:**
- `sync-gh discover` - Discover GitHub repos from Taskfile.yml remote includes
- `sync-gh poll` - Poll repositories for updates continuously
- `sync-gh poll-state` - Show current poll state (tracked repos and commit hashes)
- `sync-gh relay` - Start webhook relay with Cloudflare tunnel (zero config real-time sync)
- `sync-gh release` - Get latest release tag for a repository
- `sync-gh replay` - Replay webhook deliveries from GitHub API
- `sync-gh server` - Start a gosmee-compatible SSE server for webhook relay
- `sync-gh sse-client` - Connect to a gosmee server and forward events to local webhook handler
- `sync-gh state` - Capture or display GitHub repository state
- `sync-gh webhook` - Start webhook server
- `sync-gh webhook-add` - Configure a GitHub repo to send webhooks to a URL
- `sync-gh webhook-delete` - Delete a webhook from a GitHub repo
- `sync-gh webhook-list` - List webhooks configured on a GitHub repo

### `xplat task`

Run Taskfile tasks (embedded Task runner)

**Subcommands:**
- `task tools` - Taskfile validation and formatting tools

### `xplat ui`

Start Task UI web interface

### `xplat update`

Update xplat to the latest version

### `xplat version`

Print xplat version

