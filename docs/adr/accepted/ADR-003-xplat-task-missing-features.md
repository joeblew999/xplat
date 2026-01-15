# ADR-003: xplat Task Missing Features Analysis

## Status

**Accepted** - All critical features implemented

### Implementation Progress

| Feature | Status | PR/Commit |
|---------|--------|-----------|
| GitHub Actions annotations | ✅ Done | Added `emitCIErrorAnnotation()` |
| `--failfast` flag | ✅ Done | Added flag and `e.Failfast` |
| `--completion` flag | ✅ Done | Added flag, uses `task.Completion()` |
| `--no-status`, `--nested` | ✅ Done | Added flags, passed to `NewListOptions()` |
| `--sort` | ✅ Done | Added flag with custom sorters |
| `--disable-fuzzy` | ✅ Done | Added flag, sets `e.DisableFuzzy` |
| `--trusted-hosts` | ✅ Done | Added flag (CLI overrides defaults) |
| `--expiry` | ✅ Done | Added flag (CLI overrides defaults) |
| Output group options | ✅ Done | Added `--output-group-begin/end/error-only` |
| `CLI_ARGS_LIST` | ✅ Done | Added variable (slice of args) |
| `CLI_ASSUME_YES` | ✅ Done | Added variable |

## Context

xplat embeds Task (go-task/task) to provide a single-binary bootstrap tool. This document analyzed what features xplat was missing compared to the upstream Task source code (cloned at `.src/task/`).

### Why This Matters

1. **CI Integration** - Missing features may affect CI/CD pipelines
2. **User Experience** - Users familiar with standalone `task` may expect certain features
3. **Future Planning** - Identifies what to prioritize for xplat enhancements

---

## Implemented Features

### 1. GitHub Actions Error Annotations ✅

**Status:** When `xplat task` runs in GitHub Actions and fails, errors now appear as clickable annotations in the workflow UI.

---

### 2. `--completion` Flag ✅

**Status:** Shell completions now available via `xplat task --completion bash|zsh|fish|powershell`.

---

### 3. `--no-status` and `--nested` List Options ✅

**Status:** JSON list output now supports `--no-status` and `--nested` options.

---

### 4. `--sort` Flag ✅

**Status:** Task list ordering controllable with `--sort default|alphanumeric|none`.

---

### 5. `--disable-fuzzy` Flag ✅

**Status:** Fuzzy matching can be disabled via CLI.

---

### 6. `--failfast` Flag ✅

**Status:** Failfast mode now available for parallel task execution with `-F` or `--failfast`.

---

### 7. `--trusted-hosts` Flag ✅

**Status:** Trusted hosts can be specified via CLI (overrides xplat defaults).

---

### 8. `--expiry` Flag ✅

**Status:** Cache expiry can be controlled via CLI (overrides xplat defaults).

---

### 9. Output Group Options ✅

**Status:** Grouped output customizable with:
- `--output-group-begin` - Message before grouped output
- `--output-group-end` - Message after grouped output
- `--output-group-error-only` - Swallow output from successful tasks

---

### 10. CLI_ARGS_LIST Variable ✅

**Status:** Taskfiles can access CLI args as a list for iteration.

---

### 11. CLI_ASSUME_YES Variable ✅

**Status:** Taskfiles can check if `--yes` was passed.

---

## Not Implemented (By Design)

### `--experiments` Flag

**Reason:** xplat enables `TASK_X_REMOTE_TASKFILES=1` by default. Users rarely need to list experiments.

### `--remote-cache-dir` Flag

**Reason:** xplat uses Task's default `.task/` directory. This is sufficient for most use cases.

### `.taskrc.yml` Integration

**Reason:** xplat uses opinionated defaults baked into the binary (see ADR-002). Users CAN still use `.taskrc.yml` but xplat doesn't read it for flag defaults. This is intentional - fewer config files = simpler setup.

---

## Summary Table

| Feature | Priority | Status |
|---------|----------|--------|
| GitHub Actions annotations | HIGH | ✅ Done |
| `--failfast` | MEDIUM | ✅ Done |
| `--completion` | MEDIUM | ✅ Done |
| `--no-status`, `--nested` | MEDIUM | ✅ Done |
| `--sort` | LOW | ✅ Done |
| `--disable-fuzzy` | LOW | ✅ Done |
| `--trusted-hosts` | LOW | ✅ Done |
| `--expiry` | LOW | ✅ Done |
| Output group options | LOW | ✅ Done |
| `CLI_ARGS_LIST` | LOW | ✅ Done |
| `CLI_ASSUME_YES` | LOW | ✅ Done |
| `.taskrc.yml` integration | MEDIUM | N/A (by design) |
| `--experiments` | LOW | N/A (by design) |
| `--remote-cache-dir` | LOW | N/A (by design) |

---

## Template Functions Available

For reference, Task provides these template functions (from `.src/task/internal/templater/funcs.go`):

### Built-in Task Functions

| Function | Description |
|----------|-------------|
| `OS` | Returns `runtime.GOOS` |
| `ARCH` | Returns `runtime.GOARCH` |
| `numCPU` | Returns `runtime.NumCPU()` |
| `catLines` | Joins lines with spaces |
| `splitLines` | Splits string into lines |
| `fromSlash` | `filepath.FromSlash` |
| `toSlash` | `filepath.ToSlash` |
| `exeExt` | Returns `.exe` on Windows, empty otherwise |
| `shellQuote` / `q` | Shell-safe quoting |
| `splitArgs` | Parse shell arguments |
| `joinPath` | `filepath.Join` |
| `relPath` | `filepath.Rel` |
| `merge` | Merge maps |
| `spew` | Debug dump (spew.Sdump) |
| `fromYaml` / `mustFromYaml` | Parse YAML |
| `toYaml` / `mustToYaml` | Generate YAML |
| `uuid` | Generate UUID |
| `randIntN` | Random integer |

### Sprig Functions

Task includes all [Sprig](https://masterminds.github.io/sprig/) text template functions.

---

## References

- Task source: `.src/task/`
- xplat task implementation: `cmd/xplat/cmd/task.go`
- Task flags: `.src/task/internal/flags/flags.go`
- Task main: `.src/task/cmd/task/task.go`
- Task template functions: `.src/task/internal/templater/funcs.go`
