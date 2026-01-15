# ADR-002: Task Configuration and Automatic Remote Taskfiles

## Status

**Accepted** - Implemented via opinionated defaults in code

## Context

Task 3.45.3+ introduced `.taskrc.yml` configuration file support (verified from source at `.src/task/taskrc/ast/taskrc.go`). This enables project-level and user-level settings, including automatic remote taskfile trust.

### The Problem

1. **Standalone `task` users** - Users who run `task` directly need to set `TASK_X_REMOTE_TASKFILES=1`
2. **Interactive prompts** - Remote taskfile downloads prompt for confirmation unless `--yes` is passed
3. **Cache behavior** - Task's default cache expiry is 0 (always re-download)

### Options Considered

1. **Create `.taskrc.yml`** - Add another config file for users to maintain
2. **Opinionated defaults in code** - Bake sensible defaults into xplat's embedded Task runner

---

## Decision

**Opinionated defaults in code** - NO new config files required.

xplat embeds Task with sensible defaults that make remote taskfiles "just work" without users needing to:
- Set environment variables (`TASK_X_REMOTE_TASKFILES=1`)
- Answer confirmation prompts for trusted hosts
- Configure cache expiry
- Create `.taskrc.yml` files

Users CAN still create `.taskrc.yml` if they want to override xplat's defaults.

### xplat's Opinionated Defaults

| Setting | xplat Default | Task Default | Rationale |
|---------|---------------|--------------|-----------|
| TrustedHosts | github.com, raw.githubusercontent.com, gitlab.com | (empty) | Skip prompts for common Git hosts |
| CacheExpiry | 24h | 0 (always refetch) | Balance freshness vs. speed |
| Timeout | 30s | 10s | More forgiving for slow networks |
| Failfast | true | false | Fail early, fail fast |
| AssumeYes (in CI) | true | false | No interactive prompts in CI |

### Priority Order (lowest to highest)

1. **xplat opinionated defaults** (hardcoded in `task.go`)
2. **User's `.taskrc.yml` settings** (if they create one)
3. **CLI flags** (`--timeout`, `--offline`, etc.)

---

## Implementation

### Source of Truth: `internal/config/config.go`

All Task defaults are centralized in `internal/config/config.go`:

```go
// TaskDefaults holds xplat's opinionated defaults for the embedded Task runner.
type TaskDefaults struct {
    TrustedHosts        []string
    CacheExpiryDuration time.Duration
    Timeout             time.Duration
    Failfast            bool
}

// GetTaskDefaults returns xplat's opinionated defaults for the embedded Task runner.
func GetTaskDefaults() TaskDefaults {
    return TaskDefaults{
        TrustedHosts: []string{
            "github.com",
            "raw.githubusercontent.com",
            "gitlab.com",
        },
        CacheExpiryDuration: 24 * time.Hour,
        Timeout:             30 * time.Second,
        Failfast:            true,
    }
}

// IsCI returns true if running in a CI environment.
func IsCI() bool {
    return os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != ""
}
```

### Usage in `cmd/xplat/cmd/task.go`

```go
// Apply xplat's opinionated defaults for remote taskfiles.
defaults := config.GetTaskDefaults()
e.TrustedHosts = defaults.TrustedHosts
e.CacheExpiryDuration = defaults.CacheExpiryDuration
e.Timeout = defaults.Timeout
e.Failfast = defaults.Failfast

// Auto-approve prompts in CI environments
if config.IsCI() {
    e.AssumeYes = true
}
```

---

## Cache Behavior

### Cache Location

Task stores remote taskfiles in `.task/` directory (project root). This is correct:
- Project-local (no global state pollution)
- Usually in `.gitignore` already
- Matches standalone Task behavior

xplat does NOT change this default.

### Cache Expiry Strategy

**Current**: 24h cache expiry (reasonable balance)

**Future improvement** (not implemented): Context-aware caching based on URL patterns:

| URL Pattern | Suggested Expiry | Rationale |
|-------------|------------------|-----------|
| `raw.githubusercontent.com/.../main/...` | Short (1h) | `main` branch changes frequently |
| `raw.githubusercontent.com/.../v1.2.3/...` | Long (7d) | Tagged releases don't change |
| `github.com/.../releases/download/...` | Infinite | Release assets are immutable |
| Other HTTPS URLs | Medium (24h) | General default |

### Force Refresh

```bash
# Force re-download of remote taskfiles
xplat task --download -t https://... --list

# CI workflows should use --download for guaranteed freshness
CI=true xplat task --download build
```

---

## Verification

```bash
# 1. Remote taskfile works without prompts
xplat task -t https://raw.githubusercontent.com/joeblew999/xplat/main/taskfiles/example.yml --list
# Should NOT prompt for confirmation (github.com is trusted)

# 2. Cache is used on second run
xplat task -t https://... --list  # First run: downloads
xplat task -t https://... --list  # Second run: uses cache (within 24h)

# 3. Force refresh
xplat task --download -t https://... --list  # Forces re-download

# 4. CI auto-approve
CI=true xplat task -t https://... build  # No prompts

# 5. Offline mode works with cached taskfiles
xplat task --offline -t https://... --list  # Uses cache even if expired
```

---

## Why NOT .taskrc.yml?

1. **More knobs to turn** - Users don't need another config file
2. **Works out of the box** - No setup required
3. **xplat already handles it** - The embedded Task has good defaults
4. **User CAN override** - If they create `.taskrc.yml`, it takes precedence over xplat defaults

---

## TaskRC Specification (Reference)

For users who want to create their own `.taskrc.yml`:

**Verified from source:** `.src/task/taskrc/ast/taskrc.go`

```go
type TaskRC struct {
    Version      *semver.Version `yaml:"version"`
    Verbose      *bool           `yaml:"verbose"`
    Color        *bool           `yaml:"color"`
    DisableFuzzy *bool           `yaml:"disable-fuzzy"`
    Concurrency  *int            `yaml:"concurrency"`
    Remote       Remote          `yaml:"remote"`
    Failfast     bool            `yaml:"failfast"`
    Experiments  map[string]int  `yaml:"experiments"`
}

type Remote struct {
    Insecure     *bool          `yaml:"insecure"`
    Offline      *bool          `yaml:"offline"`
    Timeout      *time.Duration `yaml:"timeout"`
    CacheExpiry  *time.Duration `yaml:"cache-expiry"`
    CacheDir     *string        `yaml:"cache-dir"`
    TrustedHosts []string       `yaml:"trusted-hosts"`
}
```

**File search order** (from `.src/task/taskrc/taskrc.go`):
1. `$XDG_CONFIG_HOME/task/taskrc.yml` (no dot prefix)
2. `$HOME/.taskrc.yml`
3. Current directory and parent directories (`.taskrc.yml`)

Files are merged with later files overriding earlier ones.

---

## Consequences

### Positive

- Remote taskfiles "just work" without any configuration
- No new config files for users to maintain
- Sensible defaults for TrustedHosts, cache expiry, timeout
- CI environments auto-approve (no prompts)
- Users can still override via `.taskrc.yml` or CLI flags

### Negative

- None significant

### Neutral

- xplat Go code continues to set `TASK_X_REMOTE_TASKFILES=1` (required for the experiment)
- Existing Taskfiles work unchanged

---

## Future Improvements

### 1. URL-aware cache expiry
Detect pinned versions (v1.2.3) vs branches (main) and cache accordingly.

### 2. HTTP ETag support
Use conditional requests to check if remote changed before downloading.

### 3. `xplat task cache` subcommand
List cached taskfiles, show expiry, manually invalidate.

### 4. Integration with syncgh for Smart Cache Invalidation

xplat already has a `syncgh` system (`internal/syncgh/`) that can:
- **Poll** GitHub repos for changes (commit hashes, tags)
- **Receive webhooks** for real-time push notifications

**Proposed integration**:

```
┌─────────────────────────────────────────────────────────────────┐
│                     Smart Cache Flow                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Cache remote taskfile:                                      │
│     - Store in .task/                                           │
│     - Record source: github.com/owner/repo/branch/path         │
│                                                                 │
│  2. syncgh detects change (webhook or poll):                   │
│     - GitHub push event to owner/repo/branch                   │
│     - Callback triggers cache invalidation                      │
│                                                                 │
│  3. Next `xplat task` run:                                      │
│     - Cache miss (invalidated) → fresh download                │
│     - OR cache hit (not invalidated) → use cached              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Benefits over time-based expiry**:
- Instant invalidation when upstream changes (no 24h delay)
- No unnecessary re-downloads when nothing changed
- Works with long-lived services (xplat service)

**Implementation sketch**:

```go
// In syncgh webhook handler
handler.OnPushEvent(func(ctx context.Context, deliveryID string, event *github.PushEvent) error {
    repo := event.GetRepo().GetFullName()  // "owner/repo"
    ref := event.GetRef()                   // "refs/heads/main"

    // Invalidate cached taskfiles from this repo+ref
    cache.InvalidateBySource(repo, ref)
    return nil
})
```

This would be optional - if syncgh isn't running, the 24h expiry fallback still works.

---

## References

- Task Configuration Reference: https://taskfile.dev/docs/reference/config
- Remote Taskfiles Experiment: https://taskfile.dev/experiments/remote-taskfiles
- Task source: `.src/task/taskrc/ast/taskrc.go`
- xplat defaults: `internal/config/config.go` (source of truth)
- xplat task command: `cmd/xplat/cmd/task.go`
