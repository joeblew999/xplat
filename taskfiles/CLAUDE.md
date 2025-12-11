# Taskfile GOLDEN RULES

This document defines the patterns that MUST be applied consistently across ALL taskfiles.

## Taskfile Archetypes

Every Taskfile falls into one of these archetypes. Use xplat commands to see definitions:

```bash
# List all archetypes with their requirements
xplat archetype list

# Detect archetype for a file or directory
xplat archetype detect taskfiles/Taskfile.dummy.yml
xplat archetype detect taskfiles

# Explain a specific archetype in detail
xplat archetype explain tool
xplat archetype explain bootstrap
```

| Archetype | Purpose | Detection |
|-----------|---------|-----------|
| **tool** | Binary we build & release | `_BIN` + `_VERSION` + `_REPO` vars |
| **external** | External binary we install | `_BIN` + `_VERSION`, no `_REPO` |
| **builder** | Provides build infrastructure | `*_BUILD_*` vars |
| **aggregation** | Groups children via includes | `includes:` section |
| **bootstrap** | Self-bootstrapping (xplat) | `XPLAT_*` vars |
| **unknown** | Project workflow | No archetype markers |

---

## Validation Commands

Use `xplat fmt` and `xplat lint` to validate Taskfiles:

```bash
# Auto-fix formatting issues
xplat fmt

# Check formatting (for CI)
xplat fmt --check

# Lint for convention violations
xplat lint

# Lint with strict mode (warnings = errors)
xplat lint --strict
```

---

## Quick Reference

1. **Always use `{{.XPLAT_BIN}}`** - NEVER bare `xplat` in shell commands
2. **Always use `{{exeExt}}`** - All binary names: `'tool{{exeExt}}'`
3. **Use wrapper tasks** - Call `:tools:xplat:binary:install`, not xplat directly
4. **Use `status:`** - For idempotent check:deps tasks
5. **Use `deps:`** - Declarative dependencies, not imperative `task:` calls
6. **Use `toSlash`** - For paths: `"{{toSlash .ROOT_DIR}}/cmd/tool"`
7. **Quote colons** - Echo statements: `'echo "OK: done"'`

---

## Rule 1: Variable Naming Convention

**Pattern:** `<TOOLNAME>_<SUFFIX>`

| Suffix | Purpose | Example |
|--------|---------|---------|
| `_VERSION` | Tool version (from `xplat.env`) | `DUMMY_VERSION: v0.0.1` |
| `_REPO` | GitHub repo (owner/name) | `DUMMY_REPO: joeblew999/ubuntu-website` |
| `_BIN` | Binary name with `{{exeExt}}` | `DUMMY_BIN: 'dummy{{exeExt}}'` |
| `_CGO` | CGO requirement (0 or 1) | `DUMMY_CGO: '0'` |
| `_CMD` | Full command path (optional) | `DUMMY_CMD: '{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}'` |

**ALWAYS include `{{exeExt}}`** for binary names - this adds `.exe` on Windows.

---

## Rule 2: Use Variables, NEVER Bare Commands

**GOLDEN RULE: Always use `{{.XPLAT_BIN}}` variable, NEVER bare `xplat` in shell commands.**

This applies to ALL tool binaries:
- Use `{{.XPLAT_BIN}}` not `xplat`
- Use `{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}` not `dummy`
- Use `{{.TRANSLATE_CMD}}` not `translate`

**Wrong:**
```yaml
cmds:
  - 'xplat release list dummy'           # WRONG: bare xplat
  - 'dummy --version'                     # WRONG: bare tool name
```

**Correct:**
```yaml
cmds:
  - '{{.XPLAT_BIN}} release list dummy'   # CORRECT: uses variable
  - '{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}' # CORRECT: full path with var
```

---

## Rule 3: Variable Scoping Hierarchy

**Three levels of variables:**

1. **Root Taskfile.yml** - Global vars available everywhere
   - `BIN_INSTALL_DIR` - Where binaries are installed
   - `XPLAT_BIN` - Full path to xplat binary
   - `ROOT_DIR` - Project root directory

2. **Tool Taskfile vars:** - Tool-specific vars
   - `DUMMY_VERSION`, `DUMMY_BIN`, `DUMMY_CGO`, `DUMMY_REPO`
   - These are defined at the top of each tool's Taskfile

3. **Per-task vars:** - Task-scoped vars
   - Computed values, overrides
   - Example: `TEST_VERSION: '{{.VERSION | default "v0.0.1"}}'`

**Rule:** When calling another Taskfile, pass values via `vars:` block, NOT via global vars.

```yaml
# CORRECT: Pass via vars block
- task: :toolchain:golang:build
  vars:
    BIN: '{{.DUMMY_BIN}}'
    VERSION: '{{.DUMMY_VERSION}}'
```

---

## Rule 4: API Isolation via Wrapper Tasks

**For frequently-used xplat commands, use wrapper tasks for API isolation.**

Wrapper tasks in `Taskfile.xplat.yml`:
- `tools:xplat:binary:install` - wraps `xplat binary install`
- `tools:xplat:release:build` - wraps `xplat release build`
- `tools:xplat:release:list` - wraps `xplat release list`

**Usage:**
```yaml
check:deps:
  cmds:
    - task: :tools:xplat:binary:install
      vars:
        CLI_ARGS: 'dummy {{.DUMMY_VERSION}} {{.DUMMY_REPO}} --source "{{toSlash .ROOT_DIR}}/cmd/dummy"'
```

**When to use wrappers vs direct calls:**

| Pattern | Use Case | Example |
|---------|----------|---------|
| **Wrapper task** | Frequently used, API may evolve | `binary:install`, `release:build` |
| **Direct call with var** | Simple utility, rarely changes | `{{.XPLAT_BIN}} rm -rf .build` |
| **NEVER** | Bare command name | `xplat`, `dummy`, `translate` |

**Key insight:** When calling another Taskfile (like `:toolchain:golang:build`), we pass tool-specific values via the `vars:` block. This keeps the called Taskfile generic - it doesn't need to know about `DUMMY_BIN`, just `BIN`.

---

## Rule 5: Idempotency with `status:`

**Use `status:` for true idempotency - skips entirely if check passes.**

```yaml
check:deps:
  desc: Ensure binary is available
  status:
    - test -f "{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}"  # Skip if binary exists
  cmds:
    - task: :tools:xplat:binary:install
      vars: ...
```

**Behavior:**
- First run: `status:` check fails → runs install
- Second run: `status:` check passes → prints "Task is up to date", skips entirely

---

## Rule 6: Declarative Dependencies with `deps:`

**Use `deps:` for declarative dependencies, not imperative `task:` calls.**

```yaml
# CORRECT: Declarative
run:
  deps: [check:deps]
  cmds:
    - '{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}'

# AVOID: Imperative
run:
  cmds:
    - task: check:deps          # Less efficient
    - '{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}'
```

---

## Rule 7: Cross-Platform Path Handling

**Always use `toSlash` for paths passed to shell commands on Windows.**

```yaml
cmds:
  - task: :tools:xplat:binary:install
    vars:
      CLI_ARGS: 'tool {{.VERSION}} {{.REPO}} --source "{{toSlash .ROOT_DIR}}/cmd/tool"'
```

`toSlash` converts `C:\path\to\file` → `C:/path/to/file` (shell-safe).

---

## Rule 8: Quote Echo Statements with Colons

**YAML interprets colons specially - quote echo statements containing `:`**

```yaml
# WRONG: YAML error
- echo "OK: done"

# CORRECT: Quoted
- 'echo "OK: done"'
```

Inside `|` multiline blocks, quoting is not needed.

---

## Rule 9: Lifecycle Task Naming

**Standard lifecycle phases:**

| Phase | Pattern | Purpose |
|-------|---------|---------|
| `check:deps` | `<ns>:check:deps` | Ensure binary/deps available |
| `check:validate` | `<ns>:check:validate` | Smoke test the tool works |
| `check:health` | `<ns>:check:health` | External connectivity check |
| `build` | `<ns>:build` | Build for local dev |
| `run` | `<ns>:run` | Run the tool |
| `release:build` | `<ns>:release:build` | Build for release |
| `release:test` | `<ns>:release:test` | Test built binary |
| `release:publish` | `<ns>:release:publish` | Build all + publish |
| `ci:test` | `<ns>:ci:test` | Full CI test flow |

---

## Rule 10: Documentation Header

**Every Taskfile should have a header comment:**

```yaml
# <Tool Name> Tasks
#
# <Brief description>
#
# Usage:
#   task <ns>:command      Description
#   task <ns>:other        Description
#
# REQUIRES: <dependencies> (e.g., xplat, golang)

version: '3'
```

---

## Example: Complete Tool Taskfile

```yaml
# Dummy Tasks
#
# Minimal test tool for release workflow validation.
#
# Usage:
#   task dummy:build           Build locally
#   task dummy:run             Run the binary
#   task dummy:release:publish Build all platforms + publish
#
# REQUIRES: xplat (for binary management)

version: '3'

vars:
  # DUMMY_VERSION comes from xplat.env (loaded by root Taskfile dotenv)
  DUMMY_REPO: joeblew999/ubuntu-website
  DUMMY_BIN: 'dummy{{exeExt}}'
  DUMMY_CGO: '0'  # No CGO needed - can cross-compile

tasks:
  check:deps:
    desc: Ensure dummy binary is available
    status:
      - test -f "{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}"
    cmds:
      - task: :tools:xplat:binary:install
        vars:
          CLI_ARGS: 'dummy {{.DUMMY_VERSION}} {{.DUMMY_REPO}} --source "{{toSlash .ROOT_DIR}}/cmd/dummy"'

  build:
    desc: Build dummy binary locally
    cmds:
      - task: :toolchain:golang:build
        vars:
          BIN: '{{.DUMMY_BIN}}'
          VERSION: dev
          SOURCE: '{{.ROOT_DIR}}/cmd/dummy'
          CGO: '{{.DUMMY_CGO}}'

  run:
    desc: Run the dummy binary
    deps: [check:deps]
    cmds:
      - '{{.BIN_INSTALL_DIR}}/{{.DUMMY_BIN}}'

  release:build:
    desc: Build for release (current platform)
    cmds:
      - task: :toolchain:golang:build
        vars:
          BIN: '{{.DUMMY_BIN}}'
          VERSION: '{{.RELEASE_VERSION | default .DUMMY_VERSION}}'
          SOURCE: '{{.ROOT_DIR}}/cmd/dummy'
          CGO: '{{.DUMMY_CGO}}'

  release:publish:
    desc: Build all platforms and create GitHub release
    requires:
      vars: [VERSION]
    cmds:
      - task: :tools:xplat:release:build
        vars:
          CLI_ARGS: 'dummy'
      - task: :tools:gh:release:publish
        vars:
          NAME: 'dummy'
          VERSION: '{{.VERSION}}'
```
