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

## Future Enhancements

- [ ] `xplat manifest init` - scaffold new xplat.yaml
- [ ] Caching for GitHub discovery (avoid rate limits)
- [ ] Support for private repos (GitHub token)
- [ ] Dependency resolution between packages
- [ ] Version pinning and lockfiles
