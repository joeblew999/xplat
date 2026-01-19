# ADR-013: DuckDB for Local and Cloudflare R2 Iceberg Analytics

## Status

**Review** - Evaluating DuckDB as unified query engine for local development and Cloudflare edge

https://duckdb.org
https://github.com/duckdb/duckdb-wasm
https://github.com/tobilg/ducklings

## Context

xplat needs a data analytics layer that:

1. **Runs locally AND on Cloudflare** - Same queries, both environments
2. **Supports Iceberg tables** - Modern open table format for analytics
3. **Works with R2 storage** - Cloudflare's S3-compatible object storage
4. **Enables browser-based queries** - WASM for client-side analytics
5. **Integrates with existing Go services** - Embeddable or API-accessible

### The Opportunity

DuckDB is an in-process analytical database that:
- Runs anywhere: native, WASM, embedded
- Speaks SQL fluently
- Reads Parquet, CSV, JSON directly from URLs
- Now supports [Cloudflare R2 Data Catalog](https://developers.cloudflare.com/r2/data-catalog/) (DuckDB 1.3.0+)
- Writes/reads Iceberg tables (DuckDB 1.4.0+)

---

## DuckDB Deployment Options

### Option 1: Native (Local Development)

Standard DuckDB binary or Go/Python bindings.

```sql
-- Install extensions
INSTALL httpfs;
INSTALL iceberg;

-- Connect to R2
CREATE SECRET (
    TYPE r2,
    KEY_ID 'your-key-id',
    SECRET 'your-secret-key',
    ACCOUNT_ID 'your-account-id'
);

-- Query Parquet directly from R2
SELECT * FROM read_parquet('r2://bucket/data.parquet');
```

**Pros:**
- Full performance
- All extensions available
- Multi-threaded

**xplat use:** Local development, CI pipelines

### Option 2: WASM (Browser)

DuckDB compiled to WebAssembly runs in any browser.

```typescript
import * as duckdb from '@duckdb/duckdb-wasm';

const db = await duckdb.createDuckDB();
const conn = await db.connect();

// Query remote Parquet from R2
const result = await conn.query(`
  SELECT * FROM 'https://your-r2-bucket.r2.cloudflarestorage.com/data.parquet'
`);
```

**Limitations:**
- Single-threaded (default, experimental multi-thread)
- 4GB memory limit (browser may impose stricter)
- ~1.8MB gzipped WASM module
- **Cannot run in CF Workers** (1MB bundle limit, startup latency)

**xplat use:** Browser-based dashboards, client-side analytics

### Option 3: Cloudflare Containers (Server-Side)

Full DuckDB running in [Cloudflare Containers](https://developers.cloudflare.com/cloudflare-for-platforms/workers-for-platforms/reference/containers/) (beta).

The [cloudflare-ducklake](https://github.com/tobilg/cloudflare-ducklake) project demonstrates this:

```
┌─────────────────────────────────────────────────────────────┐
│                   Cloudflare Containers                      │
├─────────────────────────────────────────────────────────────┤
│  Docker Container                                            │
│  ├── DuckDB (full native)                                   │
│  ├── Hono.js API                                            │
│  ├── iceberg extension                                      │
│  └── httpfs extension                                       │
├─────────────────────────────────────────────────────────────┤
│  Endpoints:                                                  │
│  POST /query         → JSON results                         │
│  POST /streaming-query → Arrow stream                       │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                   Cloudflare R2                              │
│  ├── Data Catalog (Iceberg REST API)                        │
│  └── Object Storage (Parquet/Iceberg files)                 │
└─────────────────────────────────────────────────────────────┘
```

**Pros:**
- Full DuckDB capabilities
- Multi-threaded
- All extensions
- Persistent connection to R2

**Cons:**
- Cloudflare Containers is in beta
- Requires Docker image management
- Cold start latency

**xplat use:** Production analytics API, heavy queries

### Option 4: Ducklings (CF Workers)

[Ducklings](https://github.com/tobilg/ducklings) provides minimal DuckDB WASM builds specifically optimized for **both browsers AND Cloudflare Workers**.

```typescript
// Cloudflare Workers usage
import { init, DuckDB } from '@ducklings/workers';
import wasmModule from '@ducklings/workers/wasm';

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    await init({ wasmModule });
    const db = new DuckDB();
    const conn = db.connect();

    // Query Parquet from R2
    const rows = await conn.query<{count: number}>(`
      SELECT count(*) as count
      FROM 'https://bucket.r2.cloudflarestorage.com/data.parquet'
    `);

    conn.close();
    db.close();
    return Response.json(rows);
  }
};
```

**Key difference from standard duckdb-wasm:**
- Uses Emscripten's **Asyncify** to enable async `fetch()` in Workers
- `@ducklings/browser` - ~5.7MB gzipped (browsers only)
- `@ducklings/workers` - ~9.7MB gzipped (Workers compatible)

**R2 Secrets Support:**
```sql
-- Create R2 secret for direct bucket access
CREATE SECRET my_r2 (
  TYPE R2,
  KEY_ID 'your-r2-access-key-id',
  SECRET 'your-r2-secret-access-key',
  ACCOUNT_ID 'your-cloudflare-account-id'
);

-- Query using r2:// protocol
SELECT * FROM 'r2://bucket-name/path/to/file.parquet';
```

**Pros:**
- Runs directly in CF Workers (no containers needed)
- Native R2 secrets support
- Same API as standard DuckDB WASM
- No cold start latency of containers

**Cons:**
- ~9.7MB exceeds CF Workers free tier (3MB limit)
- Requires paid Workers plan
- Single-threaded (Workers limitation)
- No extensions (httpfs, iceberg built-in only)

**xplat use:** Lightweight edge queries, serverless analytics endpoints

---

## R2 Data Catalog Integration

Cloudflare's [R2 Data Catalog](https://developers.cloudflare.com/r2/data-catalog/) provides an Iceberg REST Catalog on top of R2.

### Connecting DuckDB to R2 Data Catalog

```sql
-- DuckDB 1.4.0+ required
INSTALL iceberg;
LOAD iceberg;

-- Create secret for R2 Data Catalog
CREATE SECRET r2_catalog (
    TYPE ICEBERG,
    TOKEN 'your-r2-api-token'
);

-- Attach the catalog
ATTACH 'r2_warehouse' AS r2lake (
    TYPE ICEBERG,
    CATALOG_URI 'https://ACCOUNT_ID.r2-data-catalog.cloudflare.com',
    WAREHOUSE 'YOUR_WAREHOUSE'
);

-- Query Iceberg table
SELECT * FROM r2lake.default.my_table;

-- Create new Iceberg table
CREATE TABLE r2lake.default.events AS
SELECT * FROM read_parquet('https://example.com/events.parquet');
```

### DuckLake: New Table Format

[DuckLake](https://ducklake.select/) (announced 2025-05-27) is DuckDB's native table format:

| Feature | Iceberg | DuckLake |
|---------|---------|----------|
| Catalog backend | REST API, Hive, etc. | PostgreSQL, MySQL |
| Designed for | Multi-engine (Spark, Trino) | DuckDB-optimized |
| Interop | Native | Iceberg import/export |
| Maturity | Production | New (2025) |

**Recommendation:** Start with Iceberg (R2 Data Catalog), evaluate DuckLake as it matures.

---

## Architecture for xplat

```
┌─────────────────────────────────────────────────────────────────┐
│                       xplat Analytics                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  LOCAL DEVELOPMENT                                              │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  DuckDB Native                                           │   │
│  │  ├── Go bindings (github.com/marcboeker/go-duckdb)      │   │
│  │  ├── CLI (duckdb)                                        │   │
│  │  └── Connects to R2 via httpfs                          │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  BROWSER (Client-Side Analytics)                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  DuckDB WASM                                             │   │
│  │  ├── @duckdb/duckdb-wasm npm package                    │   │
│  │  ├── Queries Parquet from R2 (with CORS)                │   │
│  │  └── No server required                                  │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  CLOUDFLARE (Server-Side Analytics)                            │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Option A: CF Containers (full DuckDB)                  │   │
│  │  ├── Heavy queries, multi-threaded                      │   │
│  │  └── DuckLake/Iceberg catalog                           │   │
│  │                                                          │   │
│  │  Option B: R2 SQL API (managed)                         │   │
│  │  ├── Cloudflare-managed query engine                    │   │
│  │  └── No container management                            │   │
│  │                                                          │   │
│  │  Option C: Ducklings (CF Workers) ★ RECOMMENDED         │   │
│  │  ├── @ducklings/workers package (~9.7MB)                │   │
│  │  ├── Native R2 secrets support                          │   │
│  │  └── No containers, runs in Workers (paid tier)         │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  SHARED STORAGE                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Cloudflare R2                                           │   │
│  │  ├── Object Storage (Parquet, Iceberg files)            │   │
│  │  ├── Data Catalog (Iceberg REST API)                    │   │
│  │  └── No egress fees                                      │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## xplat Relevance

| Use Case | DuckDB Role | Priority |
|----------|-------------|----------|
| **syncgh analytics** | Query GitHub event logs in Parquet | HIGH |
| **Task metrics** | Aggregate task execution data | HIGH |
| **Service logs** | Query structured logs from R2 | MEDIUM |
| **Browser dashboards** | Client-side WASM queries | MEDIUM |
| **Event sourcing** | Iceberg tables for event storage | LOW |

### High-Value Scenario: syncgh Event Analytics

```sql
-- Query GitHub events stored in R2
CREATE SECRET r2_secret (TYPE r2, ...);

-- Analyze PR merge times
SELECT
    repo,
    date_trunc('day', merged_at) as day,
    avg(merged_at - created_at) as avg_merge_time
FROM read_parquet('r2://xplat-data/github-events/*.parquet')
WHERE event_type = 'pull_request.merged'
GROUP BY 1, 2
ORDER BY 1, 2;
```

### Go Integration

```go
import "github.com/marcboeker/go-duckdb"

db, _ := sql.Open("duckdb", "")
defer db.Close()

// Install extensions
db.Exec("INSTALL httpfs; LOAD httpfs;")

// Query R2
rows, _ := db.Query(`
    SELECT * FROM read_parquet('r2://bucket/data.parquet')
`)
```

---

## WASM Constraints

### What Works

- Basic SQL queries
- Parquet/CSV/JSON from URLs
- In-memory databases
- Single-threaded execution

### Standard duckdb-wasm in CF Workers

| Constraint | Impact |
|------------|--------|
| 1MB bundle limit | WASM module is ~1.8MB gzipped |
| Startup latency | WASM compilation on cold start |
| No filesystem | Only HTTP/HTTPS data sources |
| Memory limit | 4GB max (browser may be less) |

**Verdict:** Standard `@duckdb/duckdb-wasm` does NOT work in CF Workers.

### Ducklings: The CF Workers Solution

[Ducklings](https://github.com/tobilg/ducklings) solves the CF Workers problem:

| Package | Size | Target | Async Fetch |
|---------|------|--------|-------------|
| `@duckdb/duckdb-wasm` | ~1.8MB | Browser only | N/A |
| `@ducklings/browser` | ~5.7MB | Browser | N/A |
| `@ducklings/workers` | ~9.7MB | CF Workers | Asyncify ✅ |

**Key insight:** Ducklings uses Emscripten's Asyncify to enable async `fetch()` calls within the WASM module, which is required for CF Workers.

**Paid tier required:** The ~9.7MB bundle exceeds the free tier (3MB), but works on paid Workers plans.

### Browser WASM with R2

```typescript
// Browser-side analytics
import * as duckdb from '@duckdb/duckdb-wasm';

// R2 bucket with CORS enabled
const R2_URL = 'https://your-bucket.r2.cloudflarestorage.com';

const db = await duckdb.createDuckDB();
const conn = await db.connect();

// Query Parquet directly from R2
const result = await conn.query(`
  SELECT * FROM '${R2_URL}/events/2025/*.parquet'
  WHERE event_type = 'push'
  LIMIT 100
`);
```

**CORS Required:** R2 buckets need CORS policy for browser access:
```json
{
  "CORSRules": [{
    "AllowedOrigins": ["https://your-app.com"],
    "AllowedMethods": ["GET", "HEAD"],
    "AllowedHeaders": ["*"]
  }]
}
```

---

## Comparison: DuckDB vs Alternatives

| Feature | DuckDB | ClickHouse | SQLite |
|---------|--------|------------|--------|
| In-process | ✅ | ❌ (server) | ✅ |
| WASM | ✅ | ❌ | ✅ |
| CF Workers | ✅ (ducklings) | ❌ | ❌ |
| Parquet native | ✅ | ✅ | ❌ |
| Iceberg | ✅ (extension) | ✅ | ❌ |
| R2 integration | ✅ | Manual | ❌ |
| Analytics focus | ✅ | ✅ | ❌ (OLTP) |
| Go bindings | ✅ | ✅ | ✅ |

**DuckDB wins for:** Local-first analytics with cloud data lake compatibility.

### Deployment Options Summary

| Option | Environment | Bundle Size | Multi-thread | Extensions |
|--------|-------------|-------------|--------------|------------|
| Native | Local/CI | N/A | ✅ | All |
| duckdb-wasm | Browser | ~1.8MB | ❌ | Limited |
| ducklings/browser | Browser | ~5.7MB | ❌ | Built-in |
| ducklings/workers | CF Workers | ~9.7MB | ❌ | Built-in |
| CF Containers | CF edge | Full image | ✅ | All |

**Recommendation:** Use **ducklings** for CF Workers (simpler than Containers, native R2 support).

---

## Implementation Plan

### Phase 1: Local Development

1. [ ] Add DuckDB to xplat (Go bindings or CLI)
2. [ ] Configure R2 access with httpfs
3. [ ] Create example queries for syncgh data
4. [ ] Add to Taskfile

### Phase 2: Browser Analytics

1. [ ] Integrate DuckDB WASM into Datastar UI (ADR-012)
2. [ ] Set up R2 CORS for browser access
3. [ ] Create dashboard components with client-side queries

### Phase 2.5: CF Workers with Ducklings

1. [ ] Create example CF Worker using `@ducklings/workers`
2. [ ] Configure R2 secrets for direct bucket access
3. [ ] Benchmark query performance vs CF Containers
4. [ ] Document paid tier requirements

### Phase 3: Cloudflare Containers

1. [ ] Evaluate CF Containers beta access
2. [ ] Deploy cloudflare-ducklake or custom image
3. [ ] Set up R2 Data Catalog with Iceberg

### Phase 4: Iceberg Tables

1. [ ] Create Iceberg schemas for xplat data
2. [ ] Set up write paths (from Go services)
3. [ ] Enable time-travel queries

---

## Source Analysis

**Cloned repositories:**
- `.src/duckdb-wasm/` - WebAssembly implementation
- `.src/cloudflare-ducklake/` - CF Containers + DuckLake example
- `.src/ducklings/` - Minimal DuckDB WASM for browsers AND CF Workers

---

## References

### Official
- [DuckDB](https://duckdb.org)
- [DuckDB WASM](https://duckdb.org/docs/stable/clients/wasm/overview)
- [DuckDB Iceberg Extension](https://duckdb.org/docs/stable/core_extensions/iceberg/iceberg_rest_catalogs)
- [Cloudflare R2 Import](https://duckdb.org/docs/stable/guides/network_cloud_storage/cloudflare_r2_import)

### Cloudflare
- [R2 Data Catalog](https://developers.cloudflare.com/r2/data-catalog/)
- [R2 + DuckDB Config](https://developers.cloudflare.com/r2/data-catalog/config-examples/duckdb/)
- [Cloudflare Data Platform](https://blog.cloudflare.com/cloudflare-data-platform/)

### Examples
- [ducklings](https://github.com/tobilg/ducklings) - Minimal DuckDB WASM for browsers AND CF Workers
- [cloudflare-ducklake](https://github.com/tobilg/cloudflare-ducklake) - DuckLake on CF Containers
- [cloudflare-duckdb](https://github.com/tobilg/cloudflare-duckdb) - DuckDB on CF Containers
- [DuckDB WASM + R2 Tutorial](https://andrewpwheeler.com/2025/06/29/using-duckdb-wasm-cloudflare-r2-to-host-and-query-big-data-for-almost-free/)
- [Iceberg in Browser with DuckDB-WASM](https://tobilg.com/posts/using-iceberg-catalogs-in-the-browser-with-duckdb-wasm/)

### Related ADRs
- ADR-006: CF Worker infrastructure
- ADR-012: Datastar TypeScript (browser UI for analytics)
