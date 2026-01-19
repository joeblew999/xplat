# xplat Actors

This document defines the different actors (user types) that interact with xplat.

## Actor Hierarchy

```
┌─────────────────────────────────────────────────────────────┐
│                    xplat Developer (us)                      │
│         Builds xplat binary, adds features, fixes bugs       │
│                  Repo: joeblew999/xplat                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼ produces
                    ┌─────────────────┐
                    │  xplat binary   │
                    └─────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐     ┌─────────────────────────┐
│   Package Developer     │     │       End User          │
│  Creates plat-* packages│     │  Uses plat-* projects   │
│  (plat-cms, plat-nats)  │     │  Runs tasks, services   │
└─────────────────────────┘     └─────────────────────────┘
              │                               │
              └───────────────┬───────────────┘
                              ▼
                    ┌─────────────────┐
                    │     AI IDE      │
                    │ (via MCP :8762) │
                    └─────────────────┘
```

## Actor Overview

| Actor | Description | Primary Commands |
|-------|-------------|------------------|
| **xplat Developer** | Us - builds xplat itself | `go build`, `task test`, `xplat docs all` |
| **Package Developer** | Creates plat-* packages | `xplat manifest`, `xplat gen`, `xplat release` |
| **End User** | Uses plat-* projects | `xplat task`, `xplat process`, `xplat ui`, `xplat gen` |
| **AI IDE** | Claude Code, Cursor, etc. | `xplat mcp` (HTTP API on :8762) |

---

## 1. xplat Developer (Us)

**Who:** Maintainers of the xplat tool itself.

**Repo:** `joeblew999/xplat`

**What we do:**
- Add new commands (like `ui`, `mcp`, `sync-cf`)
- Fix bugs in xplat
- Improve the developer experience
- Build and release xplat binaries

**Workflow:**
```bash
cd ~/workspace/go/src/github.com/joeblew999/xplat

# Make code changes
vim cmd/xplat/cmd/some_feature.go

# Full build with generation (recommended):
xplat internal dev build

# Quick build only:
xplat internal dev install

# Or if xplat is not installed yet (bootstrap):
go build . && ./xplat internal dev build

# Test
go test ./...

# Regenerate xplat's own documentation from code
xplat internal:docs all

# Release to users
xplat release
```

**NEVER use `go install`** - it installs to `~/go/bin` which causes conflicts.
See: `docs/adr/ADR-016-single-install-location.md`

**Internal commands:**
- `xplat internal dev build` - Full build with file generation
- `xplat internal dev install` - Quick build only
- `xplat internal dev info` - Show configuration values
- `xplat internal gen all` - Regenerate install.sh and CI action
- `xplat internal:docs all` - Regenerate README.md and Taskfile.yml

**Key insight:** A single `go build` produces the entire xplat binary with all features embedded:
- MCP server (AI IDE integration)
- Task UI (web interface)
- Docs generator (`xplat internal:docs` - for xplat's own docs)
- Gen commands (`xplat gen` - for user project files)
- Task runner (from go-task)
- Process-compose (service orchestration)
- OS utilities (cross-platform rm, cp, etc.)

**Note:** `xplat internal:docs` generates xplat's own README/Taskfile. `xplat gen` generates files for user projects.

---

## 2. Package Developer (plat-* creators)

**Who:** Developers creating reusable plat-* packages.

**Examples:** plat-cms, plat-nats, plat-hugo

**What they do:**
- Define a package manifest (`xplat.yaml`)
- Use `xplat gen` to generate project files
- Create tasks in `Taskfile.yml`
- Configure services in `process-compose.yaml`
- Publish for others to use

**Workflow:**
```bash
cd ~/workspace/plat-myservice

# Bootstrap project structure
xplat manifest bootstrap

# Define package manifest
vim xplat.yaml

# Generate files from manifest
xplat gen all          # Creates Taskfile.yml, process-compose.yaml, CI, .gitignore

# Or generate specific files
xplat gen taskfile     # Just Taskfile.yml
xplat gen process      # Just process-compose.yaml
xplat gen ci           # Just CI workflow
xplat gen gitignore    # Just .gitignore

# Test locally
xplat task build
xplat task test
xplat process

# Publish
xplat release
```

**Publishes:**
- `xplat.yaml` - Package manifest
- `Taskfile.yml` - Tasks others can include
- Binaries - Via GitHub releases
- Process definitions - For service orchestration

---

## 3. End User (plat-* users)

**Who:** Developers using plat-* projects (not building xplat or the packages).

**What they do:**
- Clone a plat-* repo
- Use `xplat gen` to regenerate files after changes
- Run tasks and services
- Use the web UI

**Workflow:**
```bash
cd ~/workspace/plat-cms

# Install xplat binary (one time)
curl -fsSL https://raw.githubusercontent.com/joeblew999/xplat/main/install.sh | bash

# Or download manually from releases:
# https://github.com/joeblew999/xplat/releases

# If xplat.yaml was updated, regenerate files
xplat gen all

# Run tasks
xplat task build
xplat task test

# Start services
xplat process          # Runs all services from process-compose.yaml

# Or use web UI
xplat ui               # Opens browser at :8760
```

**Common `xplat gen` usage:**
- After pulling changes that modified `xplat.yaml`
- After installing new packages with `xplat pkg install`
- To regenerate CI workflows after config changes

### GitHub Actions (CI/CD)

xplat is designed to be used in developers' own GitHub repos and Actions:

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      # Install xplat using the setup action (installs to ~/.local/bin)
      - uses: joeblew999/xplat/.github/actions/setup@main

      # Use xplat tasks
      - name: Build
        run: xplat task build

      - name: Test
        run: xplat task test
```

**Why use xplat in CI?**
- Same commands locally and in CI
- Cross-platform consistency (works on Linux runners)
- No need to install separate tools (Task, process-compose, etc.)
- Generated from `xplat gen ci` matches local workflow

---

## 4. AI IDE (MCP Client)

**Who:** AI assistants integrated into IDEs.

**Examples:** Claude Code, Cursor, Windsurf

**Integration:**
```bash
# MCP server runs on :8762
xplat mcp              # Standalone
xplat service start    # As part of service (with UI + process-compose)
```

**Capabilities via MCP:**
- Run tasks
- Execute OS commands
- Query project state
- Manage processes

---

## Command Ownership

| Command | xplat Dev | Package Dev | End User | Purpose |
|---------|-----------|-------------|----------|---------|
| `xplat internal dev build` | ✅ | - | - | Build and install xplat from source |
| `xplat internal gen all` | ✅ | - | - | Regenerate install.sh and CI action |
| `xplat internal:docs all` | ✅ | - | - | Regenerate xplat's own docs |
| `xplat gen all` | - | ✅ | ✅ | Generate project files from xplat.yaml |
| `xplat manifest` | - | ✅ | - | Bootstrap/validate package manifest |
| `xplat release` | ✅ | ✅ | - | Create releases |
| `xplat task` | ✅ | ✅ | ✅ | Run Taskfile tasks |
| `xplat process` | - | ✅ | ✅ | Run services |
| `xplat ui` | - | ✅ | ✅ | Web UI for tasks |
| `xplat pkg` | - | ✅ | ✅ | Package management |

---

## Port Allocation (876x range)

| Port | Service | Used By |
|------|---------|---------|
| 8760 | Task UI | End User (browser) |
| 8761 | Process Compose API | Internal |
| 8762 | MCP HTTP | AI IDE |
| 8763 | Webhook server | External services |

## Service Config (`~/.xplat/service.yaml`)

Default enables all features:

```yaml
ui: true           # Task UI on :8760
mcp: true          # MCP server on :8762
sync: true         # GitHub sync polling
```

All actors can customize via `xplat service config`.
