# ADR-009: Toolbelt Library Review

## Status

**Review** - Evaluating for potential use in xplat

## Context

[delaneyj/toolbelt](https://github.com/delaneyj/toolbelt) is a Go utility library containing common patterns used across projects. This ADR reviews its packages for potential adoption in xplat.

**Source**: Cloned to `.src/toolbelt/`

## Package Overview

| Package | Purpose | xplat Relevance |
|---------|---------|-----------------|
| `embeddednats/` | In-process NATS server | **HIGH** - Could replace external nats-server |
| `db/` | SQLite with zombiezen driver | **HIGH** - Better than modernc for some use cases |
| `natsrpc/` | Protobuf→NATS RPC generator | **MEDIUM** - Alternative to gRPC |
| `id/` | Snowflake ID generation | **MEDIUM** - Machine-local unique IDs |
| `egctx/` | Errgroup + context helpers | **LOW** - Simple, could inline |
| `protobuf/` | Proto marshal helpers | **LOW** - Simple wrappers |
| `web/` | Chi router helpers | **LOW** - Chi-specific |
| `wisshes/` | SSH automation (like Ansible) | **MEDIUM** - Infrastructure provisioning |
| `jtd/` | JSON Type Definition → Go | **LOW** - Schema-first development |
| `datalog/` | Triple-store query engine | **LOW** - Specialized use case |
| `eventbus.go` | Generic pub/sub | **MEDIUM** - In-process events |
| `pool.go` | Generic sync.Pool wrapper | **LOW** - Simple utility |
| `binary.go` | Little-endian read/write | **LOW** - Binary protocols |

---

## High-Value Packages

### 1. embeddednats - Embedded NATS Server

```go
// In-process NATS with JetStream - no external binary needed
server, err := embeddednats.New(ctx,
    embeddednats.WithDirectory("./data/nats"),
    embeddednats.WithShouldClearData(false),
)
server.WaitForServer()
nc, _ := server.Client()  // Get connected client
```

**Benefits for xplat:**
- Single binary deployment (no separate nats-server)
- Simplifies `task start` - no binary download step
- JetStream enabled by default
- Context-based lifecycle management

**Considerations:**
- Requires `github.com/nats-io/nats-server/v2` dependency (~10MB binary size increase)
- Good for development/testing, external NATS for production

### 2. db - SQLite Database Wrapper

```go
migrations, _ := db.MigrationsFromFS(migrationsFS, "migrations")
database, _ := db.NewDatabase(ctx,
    db.DatabaseWithFilename("app.sqlite"),
    db.DatabaseWithMigrations(migrations),
    db.DatabaseWithPragmas("foreign_keys = ON"),
)

// Separate read/write pools for concurrency
database.WriteTX(ctx, func(tx *sqlite.Conn) error { ... })
database.ReadTX(ctx, func(tx *sqlite.Conn) error { ... })
```

**Key features:**
- Uses `zombiezen.com/go/sqlite` (CGO-free SQLite via WASM)
- Separate read/write connection pools
- Automatic WAL mode + migrations
- Julian day timestamp helpers (SQLite-native)

**Benefits for xplat:**
- CGO-free = easier cross-compilation
- Built-in migration support from embed.FS
- Read/write pool separation is best practice

---

## Medium-Value Packages

### 3. natsrpc - NATS RPC Generator

Protobuf plugin generating NATS service stubs (alternative to gRPC):

```bash
go install github.com/delaneyj/toolbelt/natsrpc/cmd/protoc-gen-natsrpc@latest
```

**When to use**: Projects already using NATS for messaging that want RPC without adding gRPC complexity.

### 4. id - Snowflake IDs

```go
id := id.NextID()           // int64 snowflake ID
encoded := id.NextEncodedID() // Base32 string
hash := id.AliasHash("my-entity")  // Deterministic ID from string
```

**Uses:**
- zflake for ID generation
- Machine ID for node uniqueness
- xxh3 for fast hashing

### 5. wisshes - SSH Automation

Pure Go Ansible alternative:

```go
// Define infrastructure steps
steps := []wisshes.Step{
    wisshes.AptInstall("nginx"),
    wisshes.FileCopy(src, dst),
    wisshes.Command("systemctl start nginx"),
}
```

**Potential for xplat**: Remote deployment without Ansible dependency.

### 6. EventBus - Generic Pub/Sub

```go
bus := toolbelt.NewEventBusAsync[MyEvent]()
cancel := bus.Subscribe(ctx, func(e MyEvent) error { ... })
bus.Emit(ctx, event)
```

Type-safe in-process event bus with sync/async variants.

---

## Low-Value Packages (Simple Utilities)

| Package | What it does | Alternative |
|---------|--------------|-------------|
| `egctx/` | Errgroup with shared context | Inline 20 lines |
| `protobuf/` | Must-panic marshal/unmarshal | Direct proto calls |
| `web/` | Chi URL param helpers | Chi already has these |
| `binary.go` | Little-endian read/write | `encoding/binary` |
| `pool.go` | Generic sync.Pool | Direct sync.Pool |

---

## Interesting Patterns

### Datalog - Triple Store

```go
db := datalog.CreateDB(
    datalog.NewTriple("joe", "worksAt", "acme"),
    datalog.NewTriple("joe", "role", "engineer"),
    datalog.NewTriple("acme", "industry", "tech"),
)

// Query: Who works at acme?
results := db.Query([]string{"?person"},
    datalog.Pattern{"?person", "worksAt", "acme"},
)
```

Prolog-style pattern matching on triples. Useful for config relationships or dependency graphs.

### JTD (JSON Type Definition)

RFC 8927 schema → Go code generator:

```bash
jtd2go -input schema.json -output types.go -package myapp
```

Generates structs, enums, discriminated unions from JSON schemas.

---

## Recommendations

### Adopt Now

| Package | Use Case in xplat |
|---------|-------------------|
| `embeddednats/` | Replace external nats-server for `xplat service start --embedded` |
| `db/` | SQLite state storage (syncgh poll state, cache index) |

### Evaluate Later

| Package | When |
|---------|------|
| `natsrpc/` | If building NATS-based services |
| `wisshes/` | If adding remote deployment features |
| `id/` | If need distributed unique IDs |

### Skip

- `egctx/`, `protobuf/`, `web/`, `binary.go`, `pool.go` - Too simple, inline if needed
- `jtd/` - Only if adopting JSON Type Definition workflow
- `datalog/` - Specialized, unlikely to need

---

## Dependencies

toolbelt pulls significant dependencies:

```
github.com/nats-io/nats-server/v2  # For embeddednats
zombiezen.com/go/sqlite            # For db (CGO-free SQLite)
github.com/samber/lo               # Lodash-style helpers
github.com/goccy/go-json           # Fast JSON
```

**Strategy**: Import specific subpackages rather than root `toolbelt` package to minimize deps.

---

## Implementation Path

### Phase 1: Embedded NATS Option

Add `--embedded` flag to `xplat service start`:

```go
import "github.com/delaneyj/toolbelt/embeddednats"

if embedded {
    ns, _ := embeddednats.New(ctx,
        embeddednats.WithDirectory(filepath.Join(dataDir, "nats")),
    )
    // Use ns.Client() for internal connections
}
```

### Phase 2: SQLite State Storage

Replace JSON file state with SQLite:

```go
import "github.com/delaneyj/toolbelt/db"

database, _ := db.NewDatabase(ctx,
    db.DatabaseWithFilename(filepath.Join(cacheDir, "xplat.db")),
    db.DatabaseWithMigrations(migrations),
)
```

---

## References

- Source: `.src/toolbelt/` (cloned)
- Repository: https://github.com/delaneyj/toolbelt
- Go 1.25 (uses latest features like `iter.Seq`)
- zombiezen SQLite: https://pkg.go.dev/zombiezen.com/go/sqlite
