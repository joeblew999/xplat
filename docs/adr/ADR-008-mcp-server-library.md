# ADR-008: MCP Server Library Migration

## Status

**Proposed** - Evaluating official SDK

## Context

xplat has a working MCP server (`xplat mcp serve`) that exposes Taskfile tasks to AI IDEs. Currently uses `mark3labs/mcp-go` for local use on port 8762.

**Primary Goal**: Migrate to the official `modelcontextprotocol/go-sdk` for better spec compliance.

**Secondary Goal**: Enable running MCP on CF Workers (edge) using the same library.

### Key Finding

The [syumai/mcp cloudflare example](https://github.com/syumai/mcp/blob/main/cloudflare-go-mcp-example/main.go) uses:
- `github.com/modelcontextprotocol/go-sdk/mcp` - **Official MCP SDK** (not mark3labs)
- `github.com/syumai/workers` - CF Worker runtime (already using this)

```go
// CF Worker MCP pattern
server := mcp.NewServer(&mcp.Implementation{
    Name:    "xplat-mcp",
    Version: "v1.0.0",
}, nil)

mcp.AddTool(server, &mcp.Tool{
    Name:        "task-run",
    Description: "Run a Taskfile task",
}, TaskRunHandler)

// Stateless HTTP handler for CF Workers
mcpHandler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
    return server
}, &mcp.StreamableHTTPOptions{
    Stateless: true,
})

http.HandleFunc("/mcp", mcpHandler.ServeHTTP)
workers.Serve(nil)
```

### Current State

```
xplat mcp serve              # stdio (spawned by AI IDE)
xplat mcp serve --http       # HTTP server on :8762 (config.DefaultMCPPort)
xplat mcp serve --sse        # SSE server
```

Uses `github.com/mark3labs/mcp-go`.

## Decision

**Evaluate migrating to official `modelcontextprotocol/go-sdk` for both local and CF Workers.**

| Environment | Current | Proposed |
|-------------|---------|----------|
| Local | mark3labs/mcp-go | modelcontextprotocol/go-sdk |
| CF Workers | N/A | modelcontextprotocol/go-sdk + syumai/workers |

Benefits of unified library:
- Same code for local and CF Worker
- Official SDK = better spec compliance
- Proven to work with TinyGo/WASM (syumai example)

## Architecture

```
internal/mcp/                      # Shared MCP tool definitions
├── server.go                      # Tool registration (shared)
├── tools.go                       # Task execution tools
└── resources.go                   # MCP resources (docs)

cmd/xplat/cmd/mcp.go               # Local entry point
├── Uses internal/mcp/
├── stdio/HTTP/SSE transports
└── Full task execution

workers/mcp/                       # CF Worker entry point
├── Uses internal/mcp/
├── Uses internal/cfworker/
├── Stateless HTTP only
└── Proxies to local or limited tools
```

## Implementation Plan

### Phase 0: Evaluate Official SDK

1. [ ] Compare `modelcontextprotocol/go-sdk` API with current `mark3labs/mcp-go`
2. [ ] Check feature parity (tools, resources, transports)
3. [ ] Test local builds

### Phase 1: Migrate Local MCP

1. [ ] Replace mark3labs with official SDK in `cmd/xplat/cmd/mcp.go`
2. [ ] Extract shared code to `internal/mcp/`
3. [ ] Verify stdio/HTTP/SSE still work

### Phase 2: CF Worker MCP

1. [ ] Create `workers/mcp/` using shared `internal/mcp/`
2. [ ] Add stateless HTTP handler with CORS
3. [ ] Deploy to CF Workers
4. [ ] Test with remote AI IDE connections

## References

- Current MCP: `cmd/xplat/cmd/mcp.go`
- Official SDK: https://github.com/modelcontextprotocol/go-sdk
- mark3labs/mcp-go: https://github.com/mark3labs/mcp-go
- syumai CF example: https://github.com/syumai/mcp/blob/main/cloudflare-go-mcp-example/main.go
- syumai/workers: https://github.com/syumai/workers
- ADR-006: CF Worker infrastructure (`internal/cfworker/`)
