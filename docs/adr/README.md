# Architecture Decision Records (ADRs)

## Structure

```
docs/adr/
├── README.md           # This file
├── ADR-*.md            # Active/Proposed ADRs
└── accepted/           # Accepted (implemented) ADRs
    └── ADR-*.md
```

## Status Legend

| Status | Meaning |
|--------|---------|
| **Proposed** | Under review, not yet implemented |
| **Active** | Partially implemented, tracking progress |
| **Accepted** | Fully implemented, moved to `accepted/` |
| **Deprecated** | No longer applicable |

## Current ADRs

### Active

| ADR | Title | Status |
|-----|-------|--------|
| [ADR-001](ADR-001-kreuzberg-patterns.md) | Kreuzberg Patterns for Multi-Toolchain | Proposed |

### Accepted

| ADR | Title |
|-----|-------|
| [ADR-002](accepted/ADR-002-task-config-remote-taskfiles.md) | Task Config & Remote Taskfiles |
| [ADR-003](accepted/ADR-003-xplat-task-missing-features.md) | xplat Task Missing Features |
| [ADR-004](accepted/ADR-004-gosmee-integration.md) | gosmee Integration for Webhook Relay |

## Creating New ADRs

Use the format: `ADR-NNN-short-title.md`

Template:
```markdown
# ADR-NNN: Title

## Status

**Proposed** - Pending review

## Context

[Why is this decision needed?]

## Decision

[What was decided?]

## Consequences

[What are the implications?]
```
