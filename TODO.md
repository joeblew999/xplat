# xplat TODO

## Manifest System (IMPLEMENTED)

Each plat-* repo now has an `xplat.yaml` manifest that declares:
- Package identity (name, version, description, author)
- Binary source (go install, GitHub releases, npm, direct URL)
- Taskfile path for remote includes
- Process configs for process-compose
- Environment variables (required + optional with defaults)
- Build/runtime dependencies

### Commands

```bash
# Show manifest details
xplat manifest show /path/to/repo

# Validate manifest
xplat manifest validate

# Discover local manifests (plat-* directories)
xplat manifest discover -d /path/to/workspace

# Discover from GitHub
xplat manifest discover-github --owner=joeblew999

# Generate files from manifest
xplat manifest gen-env        # → .env.example
xplat manifest gen-process    # → process-compose.generated.yaml
xplat manifest gen-taskfile   # → Taskfile.generated.yml
xplat manifest gen-all        # All three

# Install binary from manifest
xplat manifest install /path/to/repo
xplat manifest install-all -d /path/to/workspace
```

### Repos with Manifests

**Core Infrastructure** (required by all plat-* systems):
- [x] plat-caddy - Custom Caddy build with Cloudflare DNS
- [x] plat-garage - Tiered storage (Local → R2 → B2)

**Applications**:
- [x] plat-rush - Push notifications (gorush wrapper)
- [x] plat-telemetry - Telemetry stack (NATS, Liftbridge, sync services)
- [x] plat-kronk - WebRTC codec experiments
- [x] plat-speech - Speech recognition
- [x] plat-bvlos - Drone operations

## Hugo Registry (DEPRECATED)

The old Hugo-based registry at ubuntu-website is being replaced by the manifest system.
Each repo now owns its own metadata via xplat.yaml.

## Packages to Move Out

These packages currently live in ubuntu-website and should move to plat-* repos:

- [ ] mailerlite → plat-mailerlite
- [ ] google → plat-google
- [ ] google-mcp-server → (already separate repo)
- [ ] cli → plat-cli (shared CLI framework)

## Next Up

### 1. `xplat manifest init` (DONE)
- [x] Interactive scaffolding for new xplat.yaml
- [x] Detect existing Taskfile.yml and suggest taskfile config
- [x] Detect go.mod and suggest binary config
- [x] Generate minimal valid manifest

### 2. GARAGE Project (plat-garage) - DONE

- [x] Create plat-garage repo
- [x] PocketBase-HA (hot tier)
- [x] R2 integration (warm tier)
- [x] B2 integration (cold tier)
- [x] Tiered storage policies
- [x] Add xplat.yaml manifest
- [x] Add unified CI workflow (.github/workflows/ci.yml)

### 2.5. Caddy Project (plat-caddy) - DONE

- [x] Create plat-caddy repo
- [x] Custom Caddy build with caddy-dns/cloudflare module
- [x] Add xplat.yaml manifest
- [x] Add unified CI workflow
- [x] Taskfile for build/test/lint

### 3. Clean Up ubuntu-website
- [ ] mailerlite → plat-mailerlite
- [ ] google → plat-google
- [ ] cli → plat-cli (shared CLI framework)
- [ ] Remove deprecated Hugo registry code

### 4. Service Mode (`xplat service`) - DONE

- [x] `xplat service install` - Install as system service (LaunchAgent/systemd)
- [x] `xplat service uninstall` - Remove system service
- [x] `xplat service start` - Start the service
- [x] `xplat service stop` - Stop the service
- [x] `xplat service status` - Check service status
- [x] `xplat service restart` - Restart the service
- [x] Runs `xplat dev` (process-compose) as subprocess
- [x] Works as user service (not root)
- [x] Cross-platform (macOS LaunchAgent, Linux systemd, Windows service)
- [x] Per-project naming (xplat-<dirname>) or custom --name

### 5. Core Packages Concept

- [ ] Add `core: true` field to xplat.yaml for infrastructure packages
- [ ] `xplat setup` command to install all core packages (caddy, garage)
- [ ] Dependency resolution: apps depend on core packages
- [ ] `xplat manifest gen-workflow` - Generate unified CI workflow from template

### 6. Workflow Generation

The manifest system should support generating GitHub Actions workflows:

```bash
xplat manifest gen-workflow    # → .github/workflows/ci.yml
```

Template pattern: Minimal workflow that delegates to Taskfile.
Same commands work locally and in CI.

## plat-* Directory Convention (IMPLEMENTED)

Standard directory structure for all plat-* repositories:

```
plat-example/
├── .src/      # Cloned upstream source code (gitignored)
├── .bin/      # Built or downloaded binaries (gitignored)
├── .data/     # Runtime data - db, cache, logs (gitignored)
├── xplat.yaml # Package manifest
└── Taskfile.yml
```

### Features

- **Automatic env vars**: `xplat task` injects `PLAT_SRC`, `PLAT_BIN`, `PLAT_DATA`, `PLAT_DIST`
- **Automatic gitignore**: `xplat manifest gen-gitignore` includes `**/.src/`, `**/.bin/`, `**/.data/`
- **Manifest support**: Add project-specific gitignore patterns via `xplat.yaml`

### Usage in plat-* repos

When running tasks via `xplat task`, these environment variables are automatically available:

```yaml
# Taskfile.yml - just use the env vars directly!
version: '3'

tasks:
  build:
    cmds:
      - mkdir -p $PLAT_BIN
      - go build -o $PLAT_BIN/mybinary ./cmd/main

  clone:
    cmds:
      - git clone https://github.com/example/repo $PLAT_SRC

  run:
    cmds:
      - $PLAT_BIN/mybinary --data-dir $PLAT_DATA
```

No Taskfile includes needed - `xplat task` provides these automatically.

### Manifest gitignore

```yaml
# xplat.yaml - add project-specific patterns
gitignore:
  patterns:
    - "pb_data/"      # PocketBase data
    - "node_modules/" # Node dependencies
    - "*.log"         # Log files
```

### Commands

```bash
xplat manifest gen-gitignore          # Generate .gitignore from manifest
xplat manifest gen-gitignore --force  # Overwrite existing
xplat manifest bootstrap              # Creates .gitignore + other files
```

## Future Enhancements

- [ ] Caching for GitHub discovery (avoid rate limits)
- [ ] Support for private repos (GitHub token)
- [ ] Dependency resolution between packages
- [ ] Version pinning and lockfiles 


---

## IDEAS (Reviewed)

### 1. .version System (from plat-telemetry) → OS Utility

**STATUS: DONE**

Added `os_version_file.go` command that reads/writes `.version` file - no git needed at OS level.

```bash
xplat version-file                    # Read .version (prints "dev" if missing)
xplat version-file -s v1.0.0          # Write v1.0.0 to .version
xplat version-file -f VERSION -s 2.0  # Write 2.0 to VERSION file
```

- [x] Create `os_version_file.go` - reads/writes `.version` file, returns "dev" if not found
- [x] Pure file-based, cross-platform, no git dependency
- [x] Useful in Taskfiles where git might not be available (Windows CI, Docker)

---

### 1b. Git Operations (no git binary required) → OS Utility

**STATUS: DONE**

Added `os_git.go` with git operations using go-git - no git binary needed on the system.

```bash
xplat git clone https://github.com/user/repo .src        # Clone (shallow)
xplat git clone https://github.com/user/repo .src v1.0.0 # Clone at tag
xplat git pull .src                                      # Pull updates
xplat git checkout .src v2.0.0                           # Checkout ref
xplat git hash .src                                      # Get commit hash
xplat git hash --full .src                               # Get full hash
xplat git tags .src                                      # List tags
xplat git branch .src                                    # Get branch name
xplat git is-repo .src                                   # Check if git repo
```

- [x] Create `internal/gitops` package (ported from plat-telemetry/sync-git)
- [x] Create `os_git.go` command with clone, pull, fetch, checkout, hash, tags, branch, is-repo
- [x] Uses go-git library - pure Go, no external git dependency
- [x] Works cross-platform (Windows, macOS, Linux) without git installed

---

### 2. sync-* Services from plat-telemetry → Port to xplat

**STATUS: TODO**

Port the working sync-* services from plat-telemetry. These are sync/integration services with working code.

**From plat-telemetry/sync-git/** (DONE - already ported):
- `xplat git clone/pull/fetch/checkout/hash/tags/branch/is-repo`

**From plat-telemetry/sync-gh/** (TODO):
```bash
xplat github state [repo]         # Capture GitHub state (workflows, releases)
xplat github check                # Check for upstream updates
xplat github poll                 # Poll upstream repos
xplat github webhook              # Start webhook server
xplat github tunnel <smee-url>    # Forward webhooks via smee.io
```

**From plat-telemetry/sync-cf/** (TODO):
```bash
xplat cloudflare tunnel [port]    # Start cloudflared quick tunnel
xplat cloudflare poll [interval]  # Poll CF audit logs
xplat cloudflare webhook [port]   # Start CF webhook server
xplat cloudflare check            # Check if cloudflared installed
xplat cloudflare install          # Install cloudflared
```

- [ ] Port `sync-gh/pkg/*` to `internal/github/`
- [ ] Port `sync-cf/pkg/*` to `internal/cloudflare/`
- [ ] Add `xplat github` command
- [ ] Add `xplat cloudflare` command
- [ ] Uses env vars: GITHUB_TOKEN, CF_ACCOUNT_ID, CF_API_TOKEN

---

### 3. jq Command → OS Utility

**STATUS: DONE**

Renamed `jq.go` to `os_jq.go` to match the OS utility pattern.

- [x] Rename `jq.go` → `os_jq.go`
- [x] Already uses pure Go (gojq library) - no external dependency
- [x] Essential for Taskfiles working with JSON cross-platform

---

### 4. GitHub CI for plat-caddy & plat-garage with Windows Support

**STATUS: DONE**

Updated CI workflows with cross-platform matrix (Linux, macOS, Windows).

- [x] Update plat-caddy CI with Windows matrix
- [x] Update plat-garage CI with Windows matrix
- [x] Uses Taskfile for cross-platform commands
- [x] Pattern: `if: runner.os != 'Windows'` / `if: runner.os == 'Windows'`