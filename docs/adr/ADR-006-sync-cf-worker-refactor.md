# ADR-006: sync-cf Worker Refactor

## Status

**Proposed** - Pending implementation

## Context

The `workers/sync-cf` Worker handles Cloudflare event aggregation (Pages deploys, alerts, logpush) and forwards them to the local `xplat sync-cf receive` endpoint.

Current implementation uses global variables and ad-hoc patterns, making testing difficult and losing state on Worker restart.

## Decision

**Refactor to use shared infrastructure in `internal/cfworker/` with build-tag separation for local vs CF Worker implementations.**

### Architecture

Each subsystem has **BOTH local AND CF Worker implementations** via build tags:

```
internal/cfworker/                 # SHARED: Reusable infrastructure
├── doc.go
├── env/                           # Environment variables
│   ├── env.go                     # Interface
│   ├── env_local.go               # //go:build !js || !wasm (os.Getenv)
│   └── env_workers.go             # //go:build js && wasm (cloudflare.Getenv)
├── http/                          # HTTP client
│   ├── client.go                  # HTTPClient interface
│   ├── client_local.go            # //go:build !js || !wasm (net/http.Client)
│   └── client_workers.go          # //go:build js && wasm (fetch.Client)
├── kv/                            # Key-value state
│   ├── provider.go                # StateProvider interface
│   ├── memory.go                  # In-memory (no build tag)
│   ├── file_local.go              # //go:build !js || !wasm (file-based JSON)
│   └── kv_workers.go              # //go:build js && wasm (Cloudflare KV)
└── r2/                            # Object storage (future)
    ├── fs/                        # Bucket/filesystem (PutObject, GetObject)
    │   ├── client.go, client_local.go, client_workers.go
    ├── sql/                       # SQL queries (DuckDB local, R2 SQL remote)
    │   ├── client.go, client_local.go, client_workers.go
    └── catalog/                   # Iceberg catalog (local REST, R2 Catalog)
        ├── client.go, client_local.go, client_workers.go

workers/sync-cf/                   # SPECIFIC: sync-cf Worker
├── main.go                        # Entry point only
├── internal/server/               # Server struct, handlers, middleware
├── wrangler.toml
├── Taskfile.yml
└── go.mod                         # Imports internal/cfworker
```

### Subsystem Summary

| Package | Purpose | Local Impl | CF Worker Impl |
|---------|---------|------------|----------------|
| `env/` | Environment variables | `os.Getenv` | `cloudflare.Getenv` |
| `http/` | HTTP client | `net/http.Client` | `fetch.Client` |
| `kv/` | Key-value state | File-based JSON | Cloudflare KV |
| `r2/fs/` | Object FS (buckets) | RustFS (S3) | R2 bucket bindings |
| `r2/sql/` | SQL queries | DuckDB direct | R2 SQL API |
| `r2/catalog/` | Iceberg catalog | Local REST catalog | R2 Catalog API |

**Note:** `memory.go` in kv has NO build tag - works everywhere for testing/ephemeral state.

---

## Implementation Plan

### Phase 0: Create Shared Infrastructure

1. [ ] Create `internal/cfworker/doc.go`
2. [ ] Create `internal/cfworker/env/` with build-tag files
3. [ ] Create `internal/cfworker/http/` with HTTP client abstraction
4. [ ] Create `internal/cfworker/kv/` with state provider
5. [ ] Add unit tests

### Phase 1: Refactor Worker

1. [ ] Update `workers/sync-cf/go.mod` to import `internal/cfworker`
2. [ ] Create `workers/sync-cf/internal/server/` with Server struct
3. [ ] Move handlers and add middleware
4. [ ] Simplify `main.go` to entry point only
5. [ ] Verify TinyGo build still works

### Phase 2: KV Storage

1. [ ] Wire up `kv.StateProvider` in Worker
2. [ ] Implement `MemoryProvider` and `KVProvider`
3. [ ] Add KV namespace to `wrangler.toml`

### Phase 3: Enhanced Endpoints

1. [ ] Add `/status` endpoint with structured response
2. [ ] Improve `/metrics` with storage-backed counters
3. [ ] Add event deduplication

### Phase 4: Testing

1. [ ] Unit tests for handlers (mock storage)
2. [ ] Integration tests with `wrangler dev`

---

## Future: R2 and Iceberg Support

Cloudflare R2 supports:
- **R2 Object Storage** - S3-compatible
- **R2 SQL** - Query Parquet/Iceberg tables
- **R2 Catalog** - Iceberg catalog API

### Local Development Stack

| Subsystem | Local | CF Worker |
|-----------|-------|-----------|
| Object Storage | RustFS (S3-compatible) | R2 |
| SQL Query | DuckDB direct | R2 SQL API |
| Iceberg Catalog | Local REST catalog | R2 Catalog |

**Key insight**: DuckDB is the unified query engine - same queries work locally (via RustFS) and on CF (via R2 SQL).

---

## References

- Current Worker: `workers/sync-cf/main.go`
- syumai/workers: https://github.com/syumai/workers
- Cloudflare KV: https://developers.cloudflare.com/kv/
- Cloudflare R2: https://developers.cloudflare.com/r2/
- RustFS: https://github.com/rustfs/rustfs
