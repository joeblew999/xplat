# ADR-016: Single xplat Install Location

## Status

Accepted

## Context

xplat binaries were being installed to multiple locations:
- `~/.local/bin/xplat`
- `~/go/bin/xplat`
- `/usr/local/bin/xplat`
- `/tmp/xplat` (test copies)

This caused issues:
1. **Stale binaries** - Old versions hang around and get picked up by PATH
2. **Hung processes** - Multiple xplat instances running from different locations
3. **Port conflicts** - Multiple services trying to use 8760/8762
4. **Confusion** - `which xplat` returns different results depending on PATH order

## Decision

**Single install location: `~/.local/bin/xplat`**

This is:
- User-writable (no sudo needed)
- Standard XDG location
- Already in most users' PATH
- Cross-platform compatible

## Implementation

### 1. Centralized Configuration

All install paths are defined in `internal/config/config.go`:

```go
// XplatCanonicalBin returns the canonical xplat binary path: ~/.local/bin/xplat
func XplatCanonicalBin() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".local", "bin", "xplat")
}

// XplatStaleLocations returns paths where stale xplat binaries might exist.
func XplatStaleLocations() []string {
    home, _ := os.UserHomeDir()
    return []string{
        filepath.Join(home, "go", "bin", "xplat"),
        "/usr/local/bin/xplat",
    }
}
```

### 2. `xplat update` Always Installs to Canonical Location

The updater uses the config to install to the canonical location:

```go
// internal/updater/updater.go
func CanonicalInstallPath() (string, error) {
    return config.XplatCanonicalBin(), nil
}
```

### 3. Automatic Cleanup on Update

After installing, `xplat update` removes stale binaries from non-canonical locations:

```go
func CleanStaleBinaries() {
    for _, loc := range config.XplatStaleLocations() {
        os.Remove(loc) // Silently clean up
    }
}
```

### 4. Startup Warning

Bootstrap init warns if stale binaries exist (see `internal/bootstrap/bootstrap.go`):

```
⚠️  Stale xplat at /Users/joe/go/bin/xplat (run: rm /Users/joe/go/bin/xplat)
```

### 5. Installation by Actor

| Actor | Command | Notes |
|-------|---------|-------|
| **xplat Developer** | `xplat internal dev build` | Builds from source, regenerates files, installs to canonical location |
| **End User** | `curl -fsSL .../install.sh \| bash` | Downloads release, installs to `~/.local/bin` |
| **CI** | `uses: joeblew999/xplat/.github/actions/setup@main` | See ACTORS.md for GitHub Actions example |

**For xplat developers:**
```bash
# Full build with generation (recommended):
xplat internal dev build

# Quick build only:
xplat internal dev install

# Or if xplat not installed yet (bootstrap):
go build . && ./xplat internal dev build
```

### 6. Self-Generation

Install scripts and CI actions are generated from `config.go`:

```bash
# Generate all self-managed files:
xplat internal gen all

# Generate individual files:
xplat internal gen install   # install.sh
xplat internal gen action    # .github/actions/setup/action.yml
```

Generated files include a header:
```
# GENERATED FILE - DO NOT EDIT
# Regenerate with: xplat internal:gen <type>
# Source of truth: internal/config/config.go
```

**NEVER use `go install`** - it installs to `~/go/bin` which causes conflicts.

## Migration

For existing users:

```bash
# One-time cleanup
pkill -9 xplat
rm -f ~/go/bin/xplat /usr/local/bin/xplat /tmp/xplat*

# Ensure PATH includes ~/.local/bin
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

## References

- XDG Base Directory: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html
- ACTORS.md: Installation workflows for different users
