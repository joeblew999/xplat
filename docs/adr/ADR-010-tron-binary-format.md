# ADR-010: TRON Binary Format Review

## Status

**Review** - Evaluating for potential use in xplat

## Context

[starfederation/tron](https://github.com/starfederation/tron) is a binary format for JSON-compatible data using trie-based structures. Created by @delaneyj (same author as toolbelt).

**Source**: Cloned to `.src/tron/`

## What is TRON?

**TRie Object Notation** - A binary format that represents JSON data using:
- **HAMT** (Hash Array Mapped Trie) for maps
- **Vector Trie** for arrays

Key properties:
- JSON-compatible primitives (null, boolean, number, string, array, object)
- Canonical encoding (same logical value → same bytes)
- Copy-on-write updates without rewriting entire document
- Random access without full decode
- Append-only with historical roots (versioning built-in)

---

## Benchmarks

GeoJSON fixture on AMD Ryzen 9 6900HX:

### Decode + Read
| Format | ns/op | MB/s | B/op | allocs/op |
|--------|-------|------|------|-----------|
| **TRON** | **3,093** | **861** | **216** | **9** |
| JSON | 65,311 | 33.7 | 11,512 | 345 |
| CBOR | 63,332 | 17.3 | 10,520 | 309 |

**TRON is 21x faster than JSON for read operations.**

### Decode + Modify + Encode
| Format | ns/op | MB/s | B/op | allocs/op |
|--------|-------|------|------|-----------|
| **TRON** | **19,697** | **135** | **10,562** | **24** |
| JSON | 133,167 | 16.5 | 16,649 | 469 |
| CBOR | 84,967 | 12.9 | 11,719 | 310 |

**TRON is 6.7x faster than JSON for modify operations.**

### Size Comparison
| Format | Raw (KB) | zstd (KB) |
|--------|----------|-----------|
| TRON | 2.60 | 0.98 |
| JSON | 2.15 | 0.48 |
| CBOR | 1.07 | 0.54 |

TRON is slightly larger uncompressed but compresses well with zstd.

---

## Why TRON is Fast

### Copy-on-Write Updates

Traditional JSON: Change one field → serialize entire document.

TRON: Change one field → only rewrite nodes along the path to root.

```
Document: {"users": [{"name": "Alice"}, {"name": "Bob"}]}

Update users[1].name = "Carol":
- Only rewrite: leaf node for "Carol", path nodes up to root
- Reuse: entire users[0] subtree, all other unchanged nodes
```

### Random Access Without Full Decode

```go
// JSON: Must parse entire document
var doc map[string]any
json.Unmarshal(data, &doc)
name := doc["users"].([]any)[0].(map[string]any)["name"]

// TRON: Navigate directly to value
root := tron.FromBytes(data)
name := root.Get("users").Get(0).Get("name").String()
```

### HAMT for Maps (O(log₁₆ n) lookups)

16-way trie using xxh32 hash:
- Max depth: 8 levels (32 bits / 4 bits per level)
- Lookup: Follow hash nibbles to leaf
- Collision: Rare, handled at max depth

### Vector Trie for Arrays (O(log₁₆ n) access)

16-way trie using index bits:
- Index 0x0123 → slots [0x0, 0x1, 0x2, 0x3]
- Append: O(log n), only touches path to new leaf
- Slice: O(n), must rebuild (same as Go slices)

---

## Use Cases

### Intended For
- Wire transmission (API responses)
- JSON column blobs in databases
- KV store values (Redis, DynamoDB)
- Document versioning (append-only with history)

### Not Intended For
- Compression (pair with zstd/brotli)
- Schema validation (use JSON Schema on top)
- Custom types (use CBOR/MsgPack for richer types)
- Primary database storage

---

## xplat Relevance

| Use Case | Benefit | Priority |
|----------|---------|----------|
| Task cache metadata | Fast partial updates, versioning | **HIGH** |
| syncgh poll state | Efficient state diffs | **MEDIUM** |
| Config storage | Random access to nested values | **MEDIUM** |
| API responses | Faster than JSON for large payloads | **LOW** |

### High-Value Scenario: Task Cache Index

Current approach: JSON file for cache metadata
```json
{
  "entries": {
    "<hash1>": {"url": "...", "source_repo": "...", "expires": "..."},
    "<hash2>": {"url": "...", "source_repo": "...", "expires": "..."}
  }
}
```

Problem: Adding/updating one entry requires rewriting entire file.

TRON approach:
```go
// Update single entry - only affected path is rewritten
cache := tron.FromFile("cache.tron")
cache = cache.Set("entries", hash, entry)
cache.AppendToFile("cache.tron")  // Append-only, preserves history
```

Benefits:
- Partial updates are O(log n) not O(n)
- Built-in versioning (can roll back)
- Concurrent-safe (append-only)

---

## Features

### Core (All Implementations)
- Encode/decode scalars and trees
- JSON interop (`fromJSON`/`toJSON`)

### Go Implementation (tron-go)
- Copy-on-write update helpers
- JMESPath queries (query without full decode)
- JSON Merge Patch (RFC 7386)
- JSON Schema validation (draft 2020-12)

### Other Implementations
| Feature | tron-go | tron-ts | tron-rust |
|---------|---------|---------|-----------|
| Core encode/decode | ✅ | ✅ | 🚧 |
| JSON interop | ✅ | ✅ | 🚧 |
| Copy-on-write | ✅ | ❌ | 🚧 |
| JMESPath | ✅ | ❌ | 🚧 |
| Merge Patch | ✅ | ❌ | 🚧 |

---

## Binary Format Summary

### Type Tags (3 bits)
| Type | Tag | Payload |
|------|-----|---------|
| nil | 0b000 | None |
| bit | 0b001 | Boolean in bit 3 |
| i64 | 0b010 | 8 bytes little-endian |
| f64 | 0b011 | 8 bytes IEEE-754 |
| txt | 0b100 | Length + UTF-8 |
| bin | 0b101 | Length + raw bytes |
| arr | 0b110 | Vector trie node |
| map | 0b111 | HAMT node |

### Document Layout
```
[Header: "TRON" magic (4 bytes)]
[Nodes: depth-first post-order]
[Footer: root_addr (4 bytes) + prev_root (4 bytes)]
```

### Copy-on-Write
```
Original: [Header][Nodes A][Footer → A]
After update: [Header][Nodes A][New Nodes B][Footer → B, prev=A]
```

Old roots remain accessible for history traversal.

---

## Comparison with Alternatives

| Format | Random Access | Partial Update | Versioning | Size |
|--------|---------------|----------------|------------|------|
| **TRON** | ✅ Yes | ✅ O(log n) | ✅ Built-in | Medium |
| JSON | ❌ Full parse | ❌ O(n) | ❌ No | Small |
| CBOR | ❌ Full parse | ❌ O(n) | ❌ No | Smallest |
| Protocol Buffers | ✅ Yes | ❌ Full encode | ❌ No | Small |
| FlatBuffers | ✅ Yes | ❌ No updates | ❌ No | Small |
| Cap'n Proto | ✅ Yes | ❌ No updates | ❌ No | Small |

TRON's unique value: **mutable with efficient partial updates + versioning**.

---

## Recommendations

### Adopt For
1. **Cache metadata** - Partial updates, versioning, concurrent-safe
2. **State files** - syncgh poll state, project registry
3. **Large config** - Random access to nested values

### Keep JSON For
1. Human-readable configs (xplat.yaml, Taskfile.yml)
2. API responses where clients expect JSON
3. Small files where rewrite cost is negligible

### Implementation Path

**Phase 1**: Use for Task cache index
```go
import "github.com/starfederation/tron-go"

// Replace .xplat-cache-index.json with .xplat-cache-index.tron
cache := tron.NewMap()
cache = cache.Set("entries", hash, entry)
```

**Phase 2**: Use for syncgh state
```go
// Replace syncgh-poll-state.json with syncgh-poll-state.tron
state := tron.FromFile(statePath)
state = state.Set("repos", repo, newHash)
state.AppendToFile(statePath)
```

---

## References

- Repository: https://github.com/starfederation/tron
- Go implementation: https://github.com/starfederation/tron-go
- Rust implementation: https://github.com/oliverlambson/tron-rust
- Spec: `.src/tron/SPEC.md`
- Primer: `.src/tron/PRIMER.md`
- Author: @delaneyj (also author of toolbelt - ADR-009)
