# ADR-015: Hurl for API Testing

## Status

Proposed

## Context

xplat projects often include HTTP APIs (MCP server, webhooks, web UIs). Currently there's no standardized way to test these endpoints.

[Hurl](https://hurl.dev) is a command-line tool for running HTTP requests defined in plain text format. It's designed for testing APIs and is particularly good for:
- Integration testing of REST APIs
- Testing MCP endpoints
- Validating webhook handlers
- CI/CD pipeline testing

## Problem

1. No standard way to test xplat HTTP endpoints (MCP, UI, webhooks)
2. Manual curl commands in scripts are hard to maintain
3. Need repeatable, CI-friendly API tests
4. MCP protocol validation requires structured request/response testing

## Proposal

Integrate Hurl as the standard API testing tool for xplat projects.

### Hurl File Example

```hurl
# Test MCP server health
GET http://localhost:8762/health
HTTP 200

# Test MCP tools/list endpoint
POST http://localhost:8762/mcp
Content-Type: application/json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list"
}
HTTP 200
[Asserts]
jsonpath "$.result.tools" exists

# Test task execution via MCP
POST http://localhost:8762/mcp
Content-Type: application/json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "build",
    "arguments": {}
  }
}
HTTP 200
[Asserts]
jsonpath "$.result" exists
```

### Integration Points

1. **`xplat pkg install hurl`** - Install Hurl binary via xplat package manager
2. **`xplat gen hurl`** - Generate starter .hurl files from xplat.yaml
3. **Taskfile task** - `task test:api` runs Hurl tests
4. **CI workflow** - Include Hurl tests in generated CI

### Directory Convention

```
project/
├── tests/
│   └── api/
│       ├── mcp.hurl        # MCP endpoint tests
│       ├── webhooks.hurl   # Webhook handler tests
│       └── ui.hurl         # Web UI API tests
├── Taskfile.yml
└── xplat.yaml
```

### xplat.yaml Integration

```yaml
testing:
  hurl:
    files:
      - tests/api/*.hurl
    variables:
      MCP_PORT: 8762
      UI_PORT: 8760
```

## Benefits

1. **Plain text format** - Easy to read, write, and version control
2. **Built-in assertions** - JSONPath, XPath, regex, status codes
3. **Variables** - Parameterize tests for different environments
4. **CI-friendly** - Exit codes, JUnit output, HTML reports
5. **No runtime dependencies** - Single binary, like xplat itself
6. **Captures** - Chain requests using values from previous responses

## Implementation Plan

### Phase 1: Package Support
- [ ] Add Hurl to xplat package registry
- [ ] `xplat pkg install hurl` downloads platform-appropriate binary

### Phase 2: Generation
- [ ] `xplat gen hurl` creates starter test files
- [ ] Generate MCP endpoint tests from registered tools
- [ ] Add `test:api` task to generated Taskfile

### Phase 3: CI Integration
- [ ] Include Hurl tests in `xplat gen ci` output
- [ ] Support test reports (JUnit XML, HTML)

## Alternatives Considered

| Tool | Pros | Cons |
|------|------|------|
| curl scripts | Ubiquitous | Verbose, hard to assert |
| Postman/Insomnia | GUI, collections | Not CLI-native, requires account |
| k6 | Load testing | Overkill for integration tests |
| httpie | Clean syntax | No built-in assertions |
| **Hurl** | Plain text, assertions, CI-native | Less known |

## References

- Hurl: https://hurl.dev
- Hurl GitHub: https://github.com/Orange-OpenSource/hurl
- MCP Protocol: https://modelcontextprotocol.io
- Current MCP implementation: `cmd/xplat/cmd/mcp.go`
