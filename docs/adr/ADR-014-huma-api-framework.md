# ADR-014: Consider Huma for API-driven Architecture

## Status
Proposed

## Context

Currently xplat uses:
- Cobra for CLI commands
- Custom HTTP handlers for MCP, UI, sync endpoints

The plat-geo project uses **Huma** (https://huma.rocks/) which provides:
- OpenAPI 3.1 spec generation from Go code
- Automatic request/response validation
- CLI integration (commands become API endpoints)
- Multiple router support (chi, gin, fiber, echo)

## Problem

As xplat grows, we may need:
1. A consistent API layer across all HTTP endpoints (MCP, UI, sync, webhooks)
2. OpenAPI documentation for integrations
3. Better request validation
4. Ability to expose CLI commands as API endpoints

## Proposal

Evaluate using Huma to wrap xplat's functionality:

```go
// Example: Task execution as API endpoint
type RunTaskInput struct {
    TaskName string `path:"task" doc:"Task name to run"`
    Args     []string `query:"args" doc:"Task arguments"`
}

type RunTaskOutput struct {
    Body struct {
        ExitCode int    `json:"exit_code"`
        Output   string `json:"output"`
    }
}

func (s *Server) RunTask(ctx context.Context, input *RunTaskInput) (*RunTaskOutput, error) {
    // Execute task via xplat task runner
}
```

## Benefits

1. **Unified API** - All endpoints follow same patterns
2. **Auto-documentation** - OpenAPI spec generated from code
3. **Validation** - Request validation built-in
4. **CLI/API parity** - Same logic accessible via CLI and API
5. **Proven** - Already working in plat-geo

## Considerations

1. **Migration effort** - Need to refactor existing endpoints
2. **Dependencies** - Adds huma as a dependency
3. **Learning curve** - Team needs to learn huma patterns

## Decision

TBD - needs further evaluation of:
- Which endpoints would benefit most
- Impact on MCP protocol compliance
- Performance implications

## References

- Huma: https://huma.rocks/
- plat-geo implementation: (internal reference)
- Current xplat HTTP handlers: `/internal/webui/`, `/cmd/xplat/cmd/mcp.go`
