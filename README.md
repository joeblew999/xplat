# xplat

Cross-platform Taskfile bootstrapper - a single binary that embeds:
- **Task** (taskfile runner)
- **Process-Compose** (process orchestration)
- **Cross-platform utilities** (rm, cp, mv, glob, etc.)

## Installation

```bash
# Build from source
go build -o xplat ./cmd/xplat/

# Install to ~/.local/bin
task build:install
```

## Commands

### Core

| Command | Description |
|---------|-------------|
| `xplat run` | Run a managed tool |
| `xplat task` | Run Taskfile tasks (embedded Task runner) |
| `xplat version` | Print xplat version |

### Package Management

| Command | Description |
|---------|-------------|
| `xplat binary` | Binary management commands |
| `xplat pkg` | Package management from Ubuntu Software registry |

### Process

| Command | Description |
|---------|-------------|
| `xplat process` | Process orchestration (embedded process-compose) |
| `xplat process-gen` | Generate process-compose.yaml from package registry |

### Other

| Command | Description |
|---------|-------------|
| `xplat completion` | Generate the autocompletion script for the specified shell |
| `xplat docs` | Generate documentation from xplat commands |
| `xplat help` | Help about any command |
| `xplat manifest` | Work with xplat.yaml manifests |
| `xplat os` | Cross-platform OS utilities |
| `xplat release` | Release build orchestration |
| `xplat service` | Manage xplat as a system service |
| `xplat sync-cf` | Cloudflare sync operations (no wrangler CLI required) |
| `xplat sync-gh` | GitHub sync operations (no gh CLI required) |
| `xplat update` | Update xplat to the latest version |

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

### `xplat docs`

Generate documentation from xplat commands

**Subcommands:**
- `docs all` - Generate all documentation
- `docs process` - Generate process-compose.generated.yaml from registry
- `docs readme` - Generate README.generated.md from xplat commands
- `docs taskfile` - Generate Taskfile.generated.yml with xplat wrapper tasks

### `xplat help`

Help about any command

### `xplat manifest`

Work with xplat.yaml manifests

**Subcommands:**
- `manifest bootstrap` - Bootstrap a plat-* repository with standard files
- `manifest check` - Deep validation of manifest against filesystem
- `manifest discover` - Discover manifests in plat-* directories
- `manifest discover-github` - Discover manifests from GitHub plat-* repos
- `manifest gen-all` - Generate all files from manifests
- `manifest gen-env` - Generate .env.example from manifests
- `manifest gen-gitignore` - Generate .gitignore from manifest
- `manifest gen-process` - Generate process-compose.yaml from manifests
- `manifest gen-taskfile` - Generate Taskfile.yml with remote includes
- `manifest gen-workflow` - Generate unified GitHub Actions CI workflow
- `manifest init` - Initialize a new xplat.yaml manifest
- `manifest install` - Install binary from manifest
- `manifest install-all` - Install binaries from all discovered manifests
- `manifest show` - Show manifest details
- `manifest validate` - Validate an xplat.yaml manifest

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

Package management from Ubuntu Software registry

**Subcommands:**
- `pkg info` - Show package details
- `pkg install` - Install a package (binary + taskfile)
- `pkg list` - List available packages
- `pkg remove` - Remove a package (binary + taskfile include)

### `xplat process`

Process orchestration (embedded process-compose)

**Subcommands:**
- `process tools` - Process-compose validation and formatting tools

### `xplat process-gen`

Generate process-compose.yaml from package registry

**Subcommands:**
- `process-gen add` - Add a package's process to process-compose.yaml
- `process-gen generate` - Generate process-compose.yaml from all registry packages
- `process-gen list` - List packages with process configurations
- `process-gen remove` - Remove a package's process from process-compose.yaml

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
- `sync-cf tunnel` - Start cloudflared quick tunnel
- `sync-cf webhook` - Start CF webhook server
- `sync-cf worker` - Manage sync-cf Cloudflare Worker

### `xplat sync-gh`

GitHub sync operations (no gh CLI required)

**Subcommands:**
- `sync-gh poll` - Poll repositories for updates continuously
- `sync-gh release` - Get latest release tag for a repository
- `sync-gh state` - Capture or display GitHub repository state
- `sync-gh tunnel` - Forward smee.io webhooks to local server
- `sync-gh tunnel-setup` - Create smee channel and configure GitHub webhook
- `sync-gh webhook` - Start webhook server

### `xplat task`

Run Taskfile tasks (embedded Task runner)

**Subcommands:**
- `task tools` - Taskfile validation and formatting tools

### `xplat update`

Update xplat to the latest version

### `xplat version`

Print xplat version

