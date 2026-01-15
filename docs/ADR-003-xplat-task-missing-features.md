# ADR-003: xplat Task Missing Features Analysis

## Status

**Active** - Tracking implementation of missing features

### Implementation Progress

| Feature | Status | PR/Commit |
|---------|--------|-----------|
| GitHub Actions annotations | ✅ Done | Added `emitCIErrorAnnotation()` |
| `--failfast` flag | ✅ Done | Added flag and `e.Failfast` |
| `--completion` flag | 🔄 In Progress | - |
| `--no-status`, `--nested` | 🔄 In Progress | - |

## Context

xplat embeds Task (go-task/task) to provide a single-binary bootstrap tool. This document analyzes what features xplat is missing compared to the upstream Task source code (cloned at `.src/task/`).

### Why This Matters

1. **CI Integration** - Missing features may affect CI/CD pipelines
2. **User Experience** - Users familiar with standalone `task` may expect certain features
3. **Future Planning** - Identifies what to prioritize for xplat enhancements

---

## Missing Features

### 1. GitHub Actions Error Annotations ✅ IMPLEMENTED

**Upstream Task:** `.src/task/cmd/task/task.go:48-58`

**xplat:** ✅ Implemented in `cmd/xplat/cmd/task.go:186-195`

```go
func emitCIErrorAnnotation(err error) {
    if isGA, _ := strconv.ParseBool(os.Getenv("GITHUB_ACTIONS")); !isGA {
        return
    }
    if e, ok := err.(*errors.TaskRunError); ok {
        fmt.Fprintf(os.Stdout, "::error title=Task '%s' failed::%v\n", e.TaskName, e.Err)
        return
    }
    fmt.Fprintf(os.Stdout, "::error title=Task failed::%v\n", err)
}
```

**Status:** When `xplat task` runs in GitHub Actions and fails, errors now appear as clickable annotations in the workflow UI.

---

### 2. `--experiments` Flag

**Upstream Task:** `.src/task/internal/flags/flags.go:150`

```go
pflag.BoolVar(&Experiments, "experiments", false, "Lists all the available experiments and whether or not they are enabled.")
```

**xplat:** Missing flag

**Impact:** Users cannot list available Task experiments. Currently documented in xplat as "rarely used".

**Recommendation:** LOW priority - xplat enables `TASK_X_REMOTE_TASKFILES=1` by default

---

### 3. `--completion` Flag

**Upstream Task:** `.src/task/internal/flags/flags.go:121`

```go
pflag.StringVar(&Completion, "completion", "", "Generates shell completion script.")
```

And in `cmd/task/task.go:120-127`:

```go
if flags.Completion != "" {
    script, err := task.Completion(flags.Completion)
    if err != nil {
        return err
    }
    fmt.Println(script)
    return nil
}
```

**xplat:** Missing flag

**Impact:** Users cannot generate shell completions for `xplat task`. They must use standalone `task` for this.

**Recommendation:** MEDIUM priority - add `--completion` support

---

### 4. `--no-status` and `--nested` List Options

**Upstream Task:** `.src/task/internal/flags/flags.go:127-128`

```go
pflag.BoolVar(&NoStatus, "no-status", false, "Ignore status when listing tasks as JSON")
pflag.BoolVar(&Nested, "nested", false, "Nest namespaces when listing tasks as JSON")
```

**xplat:** Missing flags

**Impact:** JSON output from `xplat task --list --json` lacks these options for controlling output format.

**Recommendation:** MEDIUM priority - add for JSON list completeness

---

### 5. `--sort` Flag for Task Listing

**Upstream Task:** `.src/task/internal/flags/flags.go:125`

```go
pflag.StringVar(&TaskSort, "sort", "", "Changes the order of the tasks when listed. [default|alphanumeric|none].")
```

**xplat:** Missing flag

**Impact:** Cannot control task list ordering.

**Recommendation:** LOW priority

---

### 6. `--disable-fuzzy` Flag

**Upstream Task:** `.src/task/internal/flags/flags.go:133`

```go
pflag.BoolVar(&DisableFuzzy, "disable-fuzzy", getConfig(config, func() *bool { return config.DisableFuzzy }, false), "Disables fuzzy matching for task names.")
```

**xplat:** Missing flag

**Impact:** Cannot disable fuzzy matching for task names via CLI.

**Recommendation:** LOW priority - can be set in `.taskrc.yml`

---

### 7. `--failfast` Flag ✅ IMPLEMENTED

**Upstream Task:** `.src/task/internal/flags/flags.go:148`

**xplat:** ✅ Implemented in `cmd/xplat/cmd/task.go:142,180,316`

```go
var taskFailfast bool
// In init():
TaskCmd.Flags().BoolVarP(&taskFailfast, "failfast", "F", false, "Stop all tasks on first failure when running in parallel")
// In runTask():
e.Failfast = taskFailfast
```

**Status:** Failfast mode now available for parallel task execution with `-F` or `--failfast`.

---

### 8. `--trusted-hosts` Flag

**Upstream Task:** `.src/task/internal/flags/flags.go:164`

```go
pflag.StringSliceVar(&TrustedHosts, "trusted-hosts", getConfig(config, func() *[]string { return &config.Remote.TrustedHosts }, nil), "List of trusted hosts for remote Taskfiles (comma-separated).")
```

**xplat:** Missing flag

**Impact:** Cannot specify trusted hosts via CLI for remote taskfiles.

**Recommendation:** LOW priority - xplat uses `--yes` and enables remote taskfiles by default

---

### 9. `--expiry` and `--remote-cache-dir` Flags

**Upstream Task:** `.src/task/internal/flags/flags.go:167-168`

```go
pflag.DurationVar(&CacheExpiryDuration, "expiry", getConfig(config, func() *time.Duration { return config.Remote.CacheExpiry }, 0), "Expiry duration for cached remote Taskfiles.")
pflag.StringVar(&RemoteCacheDir, "remote-cache-dir", getConfig(config, func() *string { return config.Remote.CacheDir }, env.GetTaskEnv("REMOTE_DIR")), "Directory to cache remote Taskfiles.")
```

**xplat:** Missing flags

**Impact:** Cannot control remote taskfile cache behavior via CLI.

**Recommendation:** LOW priority - can be set in `.taskrc.yml`

---

### 10. Output Group Options

**Upstream Task:** `.src/task/internal/flags/flags.go:142-144`

```go
pflag.StringVar(&Output.Group.Begin, "output-group-begin", "", "Message template to print before a task's grouped output.")
pflag.StringVar(&Output.Group.End, "output-group-end", "", "Message template to print after a task's grouped output.")
pflag.BoolVar(&Output.Group.ErrorOnly, "output-group-error-only", false, "Swallow output from successful tasks.")
```

**xplat:** Missing flags

**Impact:** Cannot customize grouped output formatting.

**Recommendation:** LOW priority

---

### 11. `.taskrc.yml` Config Integration

**Upstream Task:** `.src/task/internal/flags/flags.go:107-108`

```go
config, _ := taskrc.GetConfig(dir)
experiments.ParseWithConfig(dir, config)
```

**xplat:** Partial - experiments are parsed but config isn't used for flag defaults

**Impact:** `.taskrc.yml` settings like `verbose`, `color`, `concurrency` don't affect xplat defaults.

**Recommendation:** MEDIUM priority - implement `taskrc.GetConfig()` integration

---

### 12. CLI_ARGS_LIST Variable

**Upstream Task:** `.src/task/cmd/task/task.go:185`

```go
specialVars.Set("CLI_ARGS_LIST", ast.Var{Value: cliArgsPostDash})
```

**xplat:** Missing - only sets `CLI_ARGS`

**Impact:** Taskfiles cannot access CLI args as a list (for iteration).

**Recommendation:** LOW priority

---

### 13. CLI_ASSUME_YES Variable

**Upstream Task:** `.src/task/cmd/task/task.go:190`

```go
specialVars.Set("CLI_ASSUME_YES", ast.Var{Value: flags.AssumeYes})
```

**xplat:** Missing

**Impact:** Taskfiles cannot check if `--yes` was passed.

**Recommendation:** LOW priority

---

## Summary Table

| Feature | Priority | Effort | Impact | Status |
|---------|----------|--------|--------|--------|
| GitHub Actions annotations | HIGH | Low | CI visibility | ✅ Done |
| `--failfast` | MEDIUM | Low | CI reliability | ✅ Done |
| `--completion` | MEDIUM | Low | User convenience | 🔄 Next |
| `--no-status`, `--nested` | MEDIUM | Low | JSON output | 🔄 Next |
| `.taskrc.yml` integration | MEDIUM | Medium | Config consistency | Pending |
| `--experiments` | LOW | Low | Debug/info | Pending |
| `--sort` | LOW | Low | Cosmetic | Pending |
| `--disable-fuzzy` | LOW | Low | Available in config | Pending |
| `--trusted-hosts` | LOW | Low | Available in config | Pending |
| `--expiry`, `--remote-cache-dir` | LOW | Low | Available in config | Pending |
| Output group options | LOW | Low | Niche use case | Pending |
| `CLI_ARGS_LIST` | LOW | Low | Niche use case | Pending |
| `CLI_ASSUME_YES` | LOW | Low | Niche use case | Pending |

---

## Implementation Recommendations

### Phase 1: High-Impact CI Features ✅ COMPLETE

1. ✅ **Add `emitCIErrorAnnotation()`** - Implemented
2. ✅ **Add `--failfast` flag** - Implemented

### Phase 2: User Experience (In Progress)

3. 🔄 **Add `--completion` flag** - Improve shell integration
4. 🔄 **Add `--no-status`, `--nested` flags** - Complete JSON output
5. **Integrate `.taskrc.yml` config** - Respect user config

### Phase 3: Completeness

6. Add remaining flags for parity

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
