# Caddy & Cloudflared Integration TODO

## The Problem: Running plat-kronk Was Painful

Running `plat-kronk` (WebRTC video conferencing) exposed real problems with xplat:

```
Browser → Cloudflared (HTTPS) → Caddy (:8089) → LiveKit (:7880) + Demo (:9080)
```

### What Went Wrong

| Problem | What Happened |
|---------|---------------|
| **Binary discovery** | Each subsystem downloads to its own `.bin/`. xplat doesn't know where binaries are. Caddy fails if binary missing. |
| **Startup ordering** | process-compose handles deps, but xplat has no visibility. Failures happen late and are confusing. |
| **Config duplication** | Caddyfile is handwritten, separate from xplat.yaml. Routes defined in two places. |
| **Tunnel URL capture** | Cloudflared prints URL to stderr. Hard to capture, display, or generate QR code. |
| **Health coordination** | process-compose waits for health, but xplat doesn't know when stack is truly ready. |
| **No awareness** | xplat treats Caddy/Cloudflared as generic processes. Doesn't understand what they *are*. |

### The Root Cause

**xplat is just a wrapper around process-compose.** It doesn't understand Caddy or Cloudflared as first-class concepts.

Current flow:
```
xplat up → process-compose up → hope binaries exist → hope configs are right
```

Desired flow:
```
xplat up
  ├── Read xplat.yaml (proxy:, tunnel:, processes:)
  ├── Ensure binaries exist (~/.xplat/bin/caddy, cloudflared)
  ├── Generate Caddyfile from proxy: routes
  ├── Generate process-compose.yml with correct paths
  ├── Start process-compose
  ├── Watch cloudflared output for tunnel URL
  └── Display: "Ready! Tunnel: https://xxx.trycloudflare.com" + QR
```

**Goal:** xplat becomes *aware* of Caddy/Cloudflared, not just a process-compose launcher.

## Solution: First-Class Caddy/Cloudflared Support

### What xplat Should Do

| Problem | Solution |
|---------|----------|
| Binary discovery | xplat installs to `~/.xplat/bin/`, injects paths into process-compose |
| Startup ordering | xplat pre-checks binaries exist before starting anything |
| Config duplication | `proxy:` in xplat.yaml → auto-generate Caddyfile |
| Tunnel URL capture | xplat parses cloudflared stderr, extracts URL |
| Health coordination | xplat shows progress: "LiveKit ✓", "Caddy ✓", "Tunnel: https://..." |
| Awareness | Caddy/Cloudflared are recognized types, not generic processes |

### How It Works

**1. Binary Management**
```bash
# xplat ensures binaries exist before process-compose starts
~/.xplat/bin/
  ├── caddy           # Downloaded once, version managed
  ├── cloudflared     # Downloaded once, version managed
  └── versions.json   # Track installed versions
```

**2. Config Generation**
```yaml
# xplat.yaml - single source of truth
proxy:
  port: 8089
  routes:
    - path: /rtc*
      upstream: localhost:7880
    - path: /*
      upstream: localhost:9080
```
↓ xplat generates ↓
```
# .xplat/Caddyfile (auto-generated, don't edit)
:8089 {
  handle /rtc* { reverse_proxy localhost:7880 }
  handle { reverse_proxy localhost:9080 }
}
```

**3. Process-Compose Integration**
```yaml
# .xplat/process-compose.yml (auto-generated)
processes:
  caddy:
    command: ~/.xplat/bin/caddy run --config .xplat/Caddyfile
    depends_on: [livekit, webrtc-demo]
  tunnel:
    command: ~/.xplat/bin/cloudflared tunnel --url localhost:8089
    depends_on: [caddy]
```

**4. Tunnel URL Capture**
```go
// xplat watches cloudflared output for the URL pattern
// INF |  https://random-words.trycloudflare.com
// Then displays it prominently + generates QR code
```

**5. Unified Status**
```
$ xplat up
Starting plat-kronk...
  ✓ LiveKit (localhost:7880)
  ✓ WebRTC Demo (localhost:9080)
  ✓ Caddy proxy (localhost:8089)
  ✓ Tunnel ready

🌐 https://bold-flower-abc123.trycloudflare.com
[QR CODE HERE]

Press Ctrl+C to stop
```

## What Do We Need?

### Use Cases to Support

1. **Local Development (Desktop)**
   - Run multiple services (API, web, admin) on different ports
   - Access them all via single HTTPS endpoint (localhost:443)
   - Access from other devices on LAN (phone, tablet for testing)

2. **Remote Access (Desktop → Internet)**
   - Share local dev with teammates/clients
   - Receive webhooks from external services (GitHub, Stripe, etc.)
   - Demo to stakeholders without deploying

3. **Cloud Deployment (Fly.io)**
   - Tunnel from Fly.io back to Cloudflare edge
   - Connect to other services in the mesh
   - No local HTTPS needed (Fly handles TLS)

4. **plat-kronk Scenario (WebRTC)**
   - LiveKit on :7880 (WebSocket + API)
   - Demo server on :9080 (token gen + UI)
   - Caddy merges them on :8089
   - Cloudflared exposes to internet
   - Must work for mobile access (QR code)

### What Each Tool Provides

| Capability | Caddy | Cloudflared |
|------------|-------|-------------|
| Local HTTPS (localhost) | ✅ | ❌ |
| LAN access (192.168.x.x) | ✅ | ❌ |
| Internet access (public URL) | ❌ | ✅ |
| Reverse proxy routing | ✅ | ❌ (single service per tunnel) |
| Webhooks from internet | ❌ | ✅ |
| Zero config public URLs | ❌ | ✅ (quick tunnels) |
| Custom domains | ✅ (local only) | ✅ (via Cloudflare DNS) |
| Works offline | ✅ | ❌ |

### Requirements Summary

**Caddy (Desktop only):**
- [ ] Route multiple local services under one port
- [ ] HTTPS with mkcert certs (trusted locally)
- [ ] LAN IP detection and cert generation
- [ ] Dynamic add/remove services
- [ ] Health checks for registered services

**Cloudflared (Desktop + Cloud):**
- [ ] Quick tunnels (no Cloudflare account needed)
- [ ] Named tunnels (persistent URLs with account)
- [ ] Auto-install binary
- [ ] Tunnel health monitoring
- [ ] Support tunneling to Caddy OR direct to service

### Typical Workflows

```
# Desktop: Local dev with HTTPS
xplat proxy start
xplat proxy add api 8080 /api/*
xplat proxy add web 3000 /*
# Now: https://localhost/api/* → :8080, https://localhost/* → :3000

# Desktop: Share with internet
xplat tunnel start 443  # Tunnel Caddy's port
# Or tunnel specific service:
xplat tunnel start 8080

# Fly.io: Cloud deployment
# Caddy not needed - Fly handles TLS
# Cloudflared connects back to other services
xplat tunnel connect <service-name>
```

### Ideal plat-kronk Config (xplat.yaml)

What we want plat-kronk's config to look like:

```yaml
apiVersion: xplat/v1
kind: Package
name: kronk

# Proxy config (embedded Caddy)
proxy:
  port: 8089
  routes:
    - path: /rtc*
      upstream: localhost:7880
    - path: /twirp/*
      upstream: localhost:7880
    - path: /*
      upstream: localhost:9080

# Tunnel config (cloudflared)
tunnel:
  enabled: true
  upstream: localhost:8089  # Tunnel the proxy port

# Process management
processes:
  livekit:
    command: ./livekit-server --config livekit.yaml
    health: http://localhost:7880/

  webrtc-demo:
    command: go run ./examples/webrtc-demo
    health: http://localhost:9080/
    depends_on: [livekit]
```

Then: `xplat up` starts everything with proper ordering:
1. Start livekit, wait for healthy
2. Start webrtc-demo, wait for healthy
3. Start embedded proxy (routes configured)
4. Start tunnel (points to proxy)

**Key insight:** Caddy and Cloudflared run as **separate processes** (not goroutines) for good reasons:
- Independent lifecycle (restart one without the other)
- Clear process boundaries
- Easier debugging (separate logs)
- Cloudflared can't be embedded anyway

So "embedded Caddy" means: xplat manages Caddy in-process, but it's still logically separate from the main xplat process-compose orchestration.

## Decision: Rethink Embedding

### Why Separate Processes Make Sense

Caddy and Cloudflared running as separate processes is actually good:
- **Independent lifecycle** - restart proxy without restarting tunnel
- **Clear boundaries** - separate logs, PIDs, resource usage
- **Debuggability** - can attach to each process independently
- **Cloudflared can't embed** - closed source Go binary

### Two Approaches

**Option A: Keep External Binaries (Current)**
- xplat auto-downloads binaries to `~/.xplat/bin/`
- process-compose manages them as child processes
- Pros: Simple, proven, works today
- Cons: Multiple downloads, version management

**Option B: Embed Caddy Only**
- Caddy runs in-process (library mode)
- Cloudflared stays external (no choice)
- Pros: One less binary, tighter integration
- Cons: Adds ~30-40MB, mixed process model

### Recommendation: Option A (External) + Better UX

Keep both as external binaries, but improve the experience:

1. **Auto-install on first use** - Already have this for cloudflared
2. **Unified version management** - Track versions in xplat.yaml
3. **Built-in health checks** - xplat knows when services are ready
4. **Config generation** - Generate Caddyfile from xplat.yaml routes

This gives us the benefits without the complexity of mixed embedding.

## What xplat Already Has

| Capability | Location | What it does |
|------------|----------|--------------|
| Cloudflared install | `internal/synccf/tunnel.go` | `InstallCloudflared()` → `~/.xplat/bin/` |
| Cloudflared tunnel | `internal/synccf/tunnel.go` | `RunTunnel()` - starts tunnel, captures URL |
| Process-compose embedded | `cmd/xplat/cmd/process.go` | `xplat process up/down` |
| Process-compose gen | `cmd/xplat/cmd/process_gen.go` + `internal/process/generate.go` | Generates from remote registry |
| Manifest types | `internal/manifest/types.go` | `Manifest.Processes` config |
| Caddy (old) | `internal/env/caddy.go` | Caddyfile generation (for different use case) |
| Paths | `internal/paths/paths.go` | `XplatBin()` → `~/.xplat/bin/` |

**The Gap:**
- `process-gen` works from **remote registry**, not local xplat.yaml
- No Caddy binary management (only cloudflared)
- No `proxy:` / `tunnel:` in xplat.yaml schema

## Implementation Plan (Minimal Changes)

### Step 1: Add Caddy install (copy synccf pattern)

**File:** `internal/synccf/caddy.go` (add to existing package, not new package)

Just copy the pattern from `tunnel.go`:
- `CheckCaddy()`, `InstallCaddy()`, `GetCaddyInfo()`
- Download to `paths.XplatBin()`

**CLI:** Add to existing `sync_cf.go` or new `caddy.go` cmd

### Step 2: Extend manifest types

**File:** `internal/manifest/types.go` - add to existing Manifest struct:

```go
type Manifest struct {
    // ... existing ...
    Proxy  *ProxyConfig  `yaml:"proxy,omitempty"`
    Tunnel *TunnelConfig `yaml:"tunnel,omitempty"`
}

type ProxyConfig struct {
    Port   int           `yaml:"port"`
    Routes []RouteConfig `yaml:"routes"`
}

type RouteConfig struct {
    Path     string `yaml:"path"`
    Upstream string `yaml:"upstream"`
}

type TunnelConfig struct {
    Enabled  bool   `yaml:"enabled"`
    Upstream string `yaml:"upstream"`
}
```

### Step 3: Extend process/generate.go

**File:** `internal/process/generate.go` - add method:

```go
// GenerateFromManifest generates process-compose.yml from local xplat.yaml
func (g *Generator) GenerateFromManifest(m *manifest.Manifest) (*ProcessComposeConfig, error) {
    config := &ProcessComposeConfig{
        Version:   "0.5",
        Processes: make(map[string]Process),
    }

    // Add user processes
    for name, proc := range m.Processes {
        config.Processes[name] = manifestProcessToProcess(proc)
    }

    // Add caddy if proxy defined
    if m.Proxy != nil && len(m.Proxy.Routes) > 0 {
        config.Processes["_caddy"] = g.caddyProcess(m.Proxy, processNames(m.Processes))
    }

    // Add tunnel if enabled
    if m.Tunnel != nil && m.Tunnel.Enabled {
        config.Processes["_tunnel"] = g.tunnelProcess(m.Tunnel)
    }

    return config, nil
}
```

### Step 4: Add Caddyfile generation

**File:** `internal/process/caddyfile.go` (same package as process gen)

```go
func GenerateCaddyfile(proxy *manifest.ProxyConfig) string { ... }
```

Keep it simple, in the same package that uses it.

### Step 5: New command: `xplat up`

**File:** `cmd/xplat/cmd/up.go`

```go
// xplat up - unified startup from xplat.yaml
// 1. Load xplat.yaml
// 2. Ensure binaries (caddy, cloudflared)
// 3. Generate .xplat/Caddyfile
// 4. Generate .xplat/process-compose.yml
// 5. Run process-compose
```

This is the main new command. Everything else is small additions to existing files.

## Summary: What to Change

| File | Change |
|------|--------|
| `internal/synccf/caddy.go` | NEW - copy tunnel.go pattern for Caddy install |
| `internal/manifest/types.go` | ADD `Proxy`, `Tunnel` fields |
| `internal/process/generate.go` | ADD `GenerateFromManifest()` method |
| `internal/process/caddyfile.go` | NEW - simple Caddyfile generation |
| `cmd/xplat/cmd/up.go` | NEW - `xplat up` command |

**That's 2 new files, 2 modified files.** Not scattered everywhere.

## Desktop vs Fly.io

| Feature | Desktop | Fly.io |
|---------|---------|--------|
| `proxy:` | Start Caddy | Skip (Fly handles TLS) |
| `tunnel:` | Expose local port | Connect outward |
| Detection | Default | `FLY_APP_NAME` env var |

## End Result for plat-kronk

**Before:**
```
plat-kronk/
  ├── caddy/Taskfile.yml         # Downloads to caddy/.bin/
  ├── cloudflared/Taskfile.yml   # Downloads to cloudflared/.bin/
  ├── examples/webrtc-demo/
  │   ├── Caddyfile              # Manual
  │   └── process-compose.yml    # Manual
```

**After:**
```
plat-kronk/
  ├── xplat.yaml                 # Single source of truth
  └── .xplat/                    # Auto-generated
      ├── Caddyfile
      └── process-compose.yml

~/.xplat/bin/                    # Global binaries
  ├── caddy
  └── cloudflared
```

**Command:**
```bash
$ xplat up
  ✓ caddy v2.10.2
  ✓ cloudflared v2025.1.2
  ✓ livekit (localhost:7880)
  ✓ webrtc-demo (localhost:9080)
  ✓ caddy (localhost:8089)
  ✓ tunnel ready

🌐 https://bold-flower-abc123.trycloudflare.com
```
