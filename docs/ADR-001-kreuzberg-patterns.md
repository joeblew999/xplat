# ADR-001: Adopting Kreuzberg Patterns for xplat Multi-Toolchain Support

## Status

**Proposed** - Pending review

## Context

We explored [Kreuzberg](https://github.com/kreuzberg-dev/kreuzberg), a polyglot document intelligence framework that supports 10+ language bindings (Python, Node, Go, Rust, Java, C#, Ruby, PHP, Elixir, WASM). Their approach to Taskfile organization, multi-language toolchain support, and GitHub Actions is sophisticated and worth evaluating for adoption in xplat.

### Current xplat Approach

xplat takes a **"single binary with embedded utilities"** approach:
- `xplat os` commands (cp, mv, rm, git, jq, envsubst, etc.) are Go implementations
- No external tool dependencies (pure Go libraries like go-git, gojq, etc.)
- Works on Windows without shell utilities
- Taskfiles at `taskfiles/` with toolchain support (golang, rust, bun)

**Key xplat os implementations** (from `internal/osutil/`, `internal/gitops/`):
- File ops: `github.com/otiai10/copy`
- Git ops: `github.com/go-git/go-git/v5` (no git binary required)
- JSON: `github.com/itchyny/gojq`
- Archives: `github.com/mholt/archives`
- Glob: `github.com/bmatcuk/doublestar/v4`
- Envsubst: `github.com/a8m/envsubst`

### Kreuzberg Approach

Kreuzberg uses a **modular Taskfile architecture**:

```
.task/
├── config/
│   ├── vars.yml          # Global variables (VERSION, OS, ARCH, BUILD_PROFILE)
│   └── platforms.yml     # Cross-platform detection (EXE_EXT, LIB_EXT, NUM_CPUS)
├── languages/            # One taskfile per language (10 files)
│   ├── python.yml        # uv, maturin, pytest, ruff, mypy
│   ├── node.yml          # pnpm, napi, biome
│   ├── rust.yml          # cargo, clippy, rustfmt
│   ├── go.yml            # CGO, golangci-lint
│   ├── java.yml          # Maven, JUnit
│   ├── csharp.yml        # dotnet CLI
│   ├── ruby.yml          # Bundle, RSpec
│   ├── php.yml           # Composer, PHPUnit
│   ├── elixir.yml        # Mix
│   └── wasm.yml          # Deno, Cloudflare Workers
├── workflows/            # Cross-language orchestration
│   ├── build.yml         # build:all, build:all:dev, build:all:ci
│   ├── test.yml          # test:all, test:all:parallel
│   ├── lint.yml
│   ├── e2e.yml
│   └── benchmark.yml
└── tools/
    ├── docs.yml
    ├── pdfium.yml
    └── version-sync.yml
```

**Key patterns:**
1. **Unified task interface** - All languages have: `install`, `build`, `build:dev`, `build:release`, `build:ci`, `test`, `lint`, `clean`, `update`
2. **Build profile chaining** - `build` delegates to `build:{{.BUILD_PROFILE | default "release"}}`
3. **Internal includes** - Config taskfiles marked `internal: true`
4. **Platform-aware commands** - `platforms: [linux, darwin]` and `platforms: [windows]` variants
5. **E2E testing per language** - Each language has `e2e:generate`, `e2e:lint`, `e2e:test`, `e2e:verify`

### Kreuzberg GitHub Actions

23 reusable actions in `.github/actions/`:
- `setup-rust`, `setup-python-env`, `setup-node-workspace`, `setup-go-cgo-env`
- `install-task`, `install-system-deps`, `free-disk-space-linux`
- `cache-pdfium`, `cache-binding-artifact`, `restore-cargo-cache`
- `build-and-cache-binding`, `build-rust-ffi`

Pattern: Each action is a composite action with platform-specific scripts in `scripts/ci/`.

---

## Decision

### What We Should Adopt

#### 1. `.task/` Directory Structure

Adopt Kreuzberg's organized structure:

```
.task/
├── config/
│   ├── vars.yml
│   └── platforms.yml
├── languages/           # Rename from taskfiles/toolchain/
│   ├── golang.yml
│   ├── rust.yml
│   ├── bun.yml
│   └── [new: python.yml, node.yml, etc.]
├── workflows/
│   ├── build.yml
│   ├── test.yml
│   └── ci.yml
└── tools/
    ├── task.yml
    ├── process-compose.yml
    ├── xplat.yml
    └── gh.yml
```

#### 2. Unified Task Interface Standard

Every language taskfile should implement:

```yaml
tasks:
  install:     # Install toolchain and dependencies
  build:       # Build (delegates to build:{{.BUILD_PROFILE}})
  build:dev:   # Debug build
  build:release: # Optimized build
  build:ci:    # CI build with debug info
  test:        # Run tests
  test:ci:     # CI tests with coverage
  lint:        # Auto-fix linting
  lint:check:  # Check-only linting
  clean:       # Clean artifacts
  update:      # Update dependencies
```

#### 3. Config Variables in Separate File

Move platform detection to `.task/config/platforms.yml`:

```yaml
vars:
  EXE_EXT:
    sh: |
      case "{{OS}}" in windows) echo ".exe";; *) echo "";; esac
  LIB_EXT:
    sh: |
      case "{{OS}}" in darwin) echo "dylib";; windows) echo "dll";; *) echo "so";; esac
  NUM_CPUS:
    sh: nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4
```

#### 4. GitHub Actions Patterns

Create reusable actions:

```
.github/actions/
├── setup-xplat/         # Install xplat binary
├── setup-toolchain/     # Install Go/Rust/Bun based on input
├── cache-go-modules/
├── cache-rust-target/
└── install-task/
```

### What xplat os Should Keep

The `xplat os` commands remain valuable for:

1. **Windows compatibility** - Pure Go implementations work without shell
2. **CI environments** - No external tool dependencies (git, jq, etc.)
3. **Docker/minimal containers** - Single binary, no install steps
4. **Taskfile portability** - Cross-platform utilities

However, we should:
- Move platform detection (`OS`, `ARCH`, `EXE_EXT`) to Taskfile vars where possible
- Keep `xplat os` for commands that need pure Go (git, jq, archives)
- Use standard shell commands when running on Unix-only

### What We Should NOT Adopt

1. **No variable prefixing** - Kreuzberg doesn't use prefixes because they have separate files. xplat's PREFIX_ pattern is still needed for subsystems that share namespace.
2. **Shell scripts for platform detection** - Keep using `xplat os` where it makes sense instead of complex bash/PowerShell conditionals.

---

## Implementation Plan

### Phase 1: Restructure Taskfiles

1. Create `.task/` directory structure
2. Add `.task/config/vars.yml` and `.task/config/platforms.yml`
3. Move `taskfiles/toolchain/` to `.task/languages/`
4. Move `taskfiles/tools/` to `.task/tools/`
5. Create `.task/workflows/build.yml`, `test.yml`, `ci.yml`
6. Update root `Taskfile.yml` includes

### Phase 2: Standardize Language Taskfiles

1. Refactor existing golang.yml, rust.yml, bun.yml to unified interface
2. Add missing tasks: `build:dev`, `build:release`, `build:ci`, `lint:check`
3. Add new language support: python.yml, node.yml

### Phase 3: GitHub Actions Refactor

1. Create `.github/actions/setup-xplat/`
2. Create `.github/actions/install-task/`
3. Refactor existing workflows to use composite actions

### Phase 4: Test with Kreuzberg

1. Try running `xplat task` inside `.src/kreuzberg/`
2. Document compatibility issues
3. Identify opportunities for xplat to support Kreuzberg's patterns

---

## Files to Modify

### New Files

- `.task/config/vars.yml`
- `.task/config/platforms.yml`
- `.task/workflows/build.yml`
- `.task/workflows/test.yml`
- `.task/workflows/ci.yml`
- `.github/actions/setup-xplat/action.yml`
- `.github/actions/install-task/action.yml`

### Modified Files

- `Taskfile.yml` - Update includes to new structure
- `taskfiles/toolchain/Taskfile.golang.yml` → `.task/languages/golang.yml`
- `taskfiles/toolchain/Taskfile.rust.yml` → `.task/languages/rust.yml`
- `taskfiles/toolchain/Taskfile.bun.yml` → `.task/languages/bun.yml`
- `taskfiles/tools/Taskfile.*.yml` → `.task/tools/*.yml`

---

## Verification

1. **Run existing tasks** - `task build`, `task test` should still work
2. **Check new structure** - `task --list` shows organized task hierarchy
3. **Test toolchain tasks** - `task golang:build:dev`, `task golang:build:release`
4. **CI validation** - GitHub Actions use new composite actions
5. **Kreuzberg test** - `cd .src/kreuzberg && xplat task --list` works

---

## Consequences

### Positive

- Cleaner organization following proven patterns
- Standardized interface for all languages
- Reusable GitHub Actions reduce duplication
- Foundation for supporting more languages
- Better separation of config, languages, workflows, tools

### Negative

- Migration effort for existing Taskfile locations
- May break existing task paths (need aliases during transition)
- More files to maintain

### Neutral

- `xplat os` commands unchanged
- CLAUDE.md validation rules may need updates for new structure

---

## References

- Kreuzberg repository: https://github.com/kreuzberg-dev/kreuzberg
- Local clone: `.src/kreuzberg/` (for exploration)
- Kreuzberg Taskfile: `.src/kreuzberg/Taskfile.yml`
- Kreuzberg Task structure: `.src/kreuzberg/.task/`
