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
