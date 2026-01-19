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

### 1. `xplat update` Always Installs to Canonical Location

The updater now always installs to `~/.local/bin/xplat` regardless of where the current binary is running from:

```go
// internal/updater/updater.go
func CanonicalInstallPath() (string, error) {
    home := os.Getenv("HOME")
    return filepath.Join(home, ".local", "bin", "xplat"), nil
}
```

### 2. Automatic Cleanup on Update

After installing, `xplat update` removes stale binaries from non-canonical locations:

```go
func CleanStaleBinaries() {
    staleLocations := []string{
        filepath.Join(home, "go", "bin", "xplat"),
        "/usr/local/bin/xplat",
    }
    for _, loc := range staleLocations {
        os.Remove(loc) // Silently clean up
    }
}
```

### 3. Startup Warning

Bootstrap init warns if stale binaries exist (see `internal/bootstrap/bootstrap.go`):

```
⚠️  Stale xplat at /Users/joe/go/bin/xplat (run: rm /Users/joe/go/bin/xplat)
```

### 4. Taskfile for xplat Developers

The generated Taskfile includes `build:clean-stale` which removes stale binaries before installing:

```yaml
build:install:
  desc: Build and install xplat to ~/.local/bin (ONLY location)
  cmds:
    - task: build:clean-stale
    - mkdir -p ~/.local/bin
    - go build -o ~/.local/bin/xplat .
```

### 5. No More go install

Avoid `go install` which puts binaries in `~/go/bin`. Use `xplat update` or `task build:install`.

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
