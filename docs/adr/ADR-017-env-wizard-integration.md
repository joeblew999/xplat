# ADR-017: Environment Configuration Wizard Integration

## Status

Proposed

## Context

xplat projects often require external service configuration (Cloudflare, GitHub tokens, etc.). Currently:

1. `internal/env/` - Contains env var definitions and validation logic
2. `internal/env/web/` - A web-based wizard UI using `go-via/via` for guided setup
3. `xplat gen env` - Generates `.env.example` from manifest

The web wizard exists but is not wired to any CLI command. Users have no way to access it.

### The Idempotency Challenge

Configuration wizards face a fundamental tension:
- First-time setup: Need to collect all values
- Subsequent runs: Values already exist in `.env`
- Partial setup: Some values exist, others missing
- Validation: Existing values may be invalid/expired

Running a wizard unconditionally risks overwriting valid configuration.

## Decision

Integrate the env wizard with the following approach:

### 1. CLI Entry Point

Add `xplat env` command group:
```
xplat env wizard     # Launch web wizard (opens browser)
xplat env check      # Validate current .env against manifest requirements
xplat env status     # Show what's configured vs missing
```

### 2. Smart Detection

Before showing any wizard step:
1. Load existing `.env` values
2. Check manifest for required env vars
3. Validate existing values (API token validity, etc.)
4. Only show steps for missing/invalid configuration

### 3. Integration Points

| Trigger | Behavior |
|---------|----------|
| `xplat env wizard` | Explicit - always available |
| `xplat gen env` | If missing required vars, suggest: "Run `xplat env wizard` to configure" |
| `xplat manifest bootstrap` | Prompt: "Configure external services now? (y/N)" |
| First `xplat process` | If manifest has processes requiring env vars, warn about missing config |

### 4. Wizard Behavior

- **Stateless**: Each page validates and saves independently
- **Skippable**: Already-configured steps show "✓ Configured" with option to reconfigure
- **Validation**: Test credentials before saving (e.g., Cloudflare API token)
- **Output**: Writes to `.env` file, never to git-tracked files

## Consequences

### Positive
- Users get guided setup for complex integrations
- Idempotent: safe to run multiple times
- Validates configuration before saving
- Web UI is more discoverable than reading docs

### Negative
- Adds complexity to CLI
- Web UI requires browser (not pure CLI)
- Must maintain wizard steps as services evolve

### Neutral
- Wizard is optional; CLI-only users can manually create `.env`

## Implementation Notes

1. `internal/env/web/` already has wizard infrastructure
2. Need to add `cmd/xplat/cmd/env.go` with command group
3. Wizard server should auto-open browser and use random port
4. Consider adding `--headless` mode for CI that just validates

## Related

- ADR-016: Single install location (canonical paths)
- `internal/env/` package for env var definitions
- `xplat.yaml` manifest env configuration
