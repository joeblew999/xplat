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

### 1. Standardized Install Task

The generated Taskfile should only use one location:

```yaml
vars:
  XPLAT_BIN: '{{.HOME}}/.local/bin/xplat{{exeExt}}'

tasks:
  build:install:
    desc: Build and install xplat
    cmds:
      - mkdir -p "{{.HOME}}/.local/bin"
      - go build -o "{{.XPLAT_BIN}}" .
```

### 2. Clean Install Script

Before installing, remove any stale binaries:

```bash
# Remove from all known locations
rm -f ~/go/bin/xplat
rm -f /usr/local/bin/xplat
rm -f /tmp/xplat*

# Install to canonical location
mkdir -p ~/.local/bin
cp ./xplat ~/.local/bin/xplat
```

### 3. PATH Check

xplat should warn if `~/.local/bin` is not in PATH:

```go
func checkPath() {
    home := os.Getenv("HOME")
    localBin := filepath.Join(home, ".local", "bin")
    if !strings.Contains(os.Getenv("PATH"), localBin) {
        fmt.Fprintf(os.Stderr, "Warning: %s is not in PATH\n", localBin)
    }
}
```

### 4. No More go install

Avoid `go install` which puts binaries in `~/go/bin`. Use explicit build + copy.

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
