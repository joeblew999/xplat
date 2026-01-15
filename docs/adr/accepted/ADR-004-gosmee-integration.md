# ADR-004: gosmee Integration for Webhook Relay

## Status

**Accepted** - Implementation complete (Option B: Cherry-Pick Key Features)

## Context

xplat needs real-time GitHub sync to invalidate Task cache when upstream repos change. We have three current approaches:

1. **Polling** (`sync-gh poll`) - Works but has 5-minute delay
2. **Direct webhook** (`sync-gh webhook`) - Requires public URL
3. **Relay** (`sync-gh relay`) - Uses cloudflared quick tunnel (URL changes on restart)

[gosmee](https://github.com/chmouel/gosmee) is a pure Go webhook relay system that provides:
- SSE-based webhook forwarding with automatic reconnection
- Event replay from GitHub API (catch missed webhooks)
- Server component for self-hosting
- Public relay at `https://hook.pipelinesascode.com`

### Why gosmee Matters

gosmee solves the "stable URL" problem:
- **smee.io** - Free but external dependency, no self-hosting
- **cloudflared quick tunnel** - Free but URL changes on restart
- **cloudflared named tunnel** - Stable URL but requires 4-step setup
- **gosmee server** - Self-hosted, stable URL, pure Go (embeddable)

---

## Existing xplat SSE Infrastructure

**IMPORTANT**: xplat already has SSE-based real-time communication via the Via web framework (`internal/webui/via_server.go`).

### Via Framework SSE Patterns

The Via framework provides SSE-based bidirectional communication for the Task UI:

```go
// Signal-based state management (SSE-backed)
output := c.Signal("")           // Create reactive signal
status := c.Signal("ready")      // State tracking
running := c.Signal(false)       // Boolean state

// Actions that trigger SSE updates
runAction := c.Action(func() {
    running.SetValue(true)
    status.SetValue("running")
    c.Sync()  // Push SSE update to all connected clients

    go func() {
        // Stream output line-by-line via SSE
        runTaskWithCallback(taskName, workDir, func(line string) {
            output.SetValue(output.String() + line)
            c.Sync()  // Real-time SSE push
        })

        running.SetValue(false)
        status.SetValue("finished")
        c.Sync()
    }()
})
```

### Via SSE Features We Already Have

| Feature | Via Implementation | gosmee Equivalent |
|---------|-------------------|-------------------|
| **Reactive signals** | `c.Signal()` | Event streaming |
| **State sync** | `c.Sync()` | SSE broadcast |
| **Real-time streaming** | Callback + Sync | SSE events |
| **Auto-refresh** | Ticker + Sync | Reconnection |
| **Bidirectional** | Actions + Signals | POST + SSE |

### Key Difference: Via vs gosmee

| Aspect | Via (xplat UI) | gosmee |
|--------|----------------|--------|
| **Purpose** | UI state management | Webhook relay |
| **SSE Direction** | Server → Browser | Server → Client process |
| **Data Model** | Reactive signals | Raw HTTP payloads |
| **Client** | JavaScript in browser | Go process |
| **Protocol** | Via's signal format | smee.io compatible |

**Insight**: Via uses SSE for browser UI updates. gosmee uses SSE for process-to-process webhook relay. These are complementary, not redundant.

---

## gosmee Analysis

### Components

| Component | File | Description |
|-----------|------|-------------|
| **Server** | `server.go` | HTTP server receiving webhooks, broadcasting via SSE |
| **Client** | `client.go` | SSE client forwarding events to local target |
| **Replay** | `replay.go` | Fetch and replay past webhook deliveries from GitHub API |
| **Hook List** | `hook_list.go`, `interface.go` | List hooks/deliveries for repos and orgs |

### Key Features

#### 1. Server (`gosmee server`)

```go
// Receives webhooks at POST /{channel}
// Broadcasts to SSE clients at GET /events/{channel}
// Features:
- Webhook signature validation (GitHub, GitLab, Bitbucket, Gitea)
- IP allowlist filtering
- TLS/Let's Encrypt support
- Max body size limits
- Random channel generation
```

**Overlap with xplat:** Our `syncgh.WebhookServer` does similar webhook receiving but lacks SSE broadcast.

#### 2. Client (`gosmee client`)

```go
// Connects to SSE endpoint
// Forwards events to local target URL
// Features:
- Automatic reconnection with exponential backoff
- Event filtering (ignore specific event types)
- Payload saving to disk for replay
- Health endpoint for K8s probes
- Version compatibility checking
```

**Overlap with xplat:** Our `syncgh.SSEClient` (just added) does similar SSE → local forwarding but lacks:
- Payload saving
- Event filtering
- Health endpoint
- Exponential backoff (we use fixed 5s retry)

#### 3. Replay (`gosmee replay`)

```go
// Fetches past webhook deliveries from GitHub API
// Replays them to local target
// Features:
- List hooks and their IDs
- List deliveries with timestamps
- Replay since specific time
- Works at repo OR org level
```

**No overlap with xplat:** This is unique functionality we don't have.

### gosmee Dependencies

```
github.com/google/go-github/v57     # We use v80/v81
github.com/r3labs/sse/v2            # SSE client/server
github.com/go-chi/chi/v5            # HTTP router
github.com/urfave/cli/v2            # CLI framework (we use cobra)
github.com/lmittmann/tint           # Colored logging
github.com/mgutz/ansi               # ANSI colors
golang.org/x/crypto/acme/autocert   # Let's Encrypt
```

---

## Integration Options

### Option A: Embed gosmee as Library

**Approach:** Import gosmee package and use its types/functions directly.

**Pros:**
- Full feature access (replay, server, client)
- Maintained upstream
- No code duplication

**Cons:**
- go-github version mismatch (v57 vs v80/v81)
- CLI framework mismatch (urfave/cli vs cobra)
- Pulls in dependencies we may not need
- gosmee's API is CLI-focused, not library-focused

**Effort:** Medium - Need to wrap gosmee's CLI-oriented code

### Option B: Cherry-Pick Key Features

**Approach:** Copy specific functionality we need, adapt to our patterns.

**Features to cherry-pick:**
1. **Replay** - Fetch and replay past webhook deliveries
2. **Server improvements** - SSE broadcast, signature validation
3. **Client improvements** - Exponential backoff, payload saving

**Pros:**
- Use our existing go-github version
- Integrates with our CLI (cobra)
- No version conflicts
- Only pull what we need

**Cons:**
- Code to maintain
- May diverge from upstream improvements

**Effort:** Medium - Selective copying and adaptation

### Option C: Coexist (Run gosmee Binary)

**Approach:** Install and run gosmee as external binary, like we do with cloudflared.

**Pros:**
- Zero integration effort
- Always get upstream improvements
- Clear separation of concerns

**Cons:**
- Another binary to install/manage
- Can't embed in xplat binary
- More moving parts for users

**Effort:** Low - Just add install/run commands

### Option D: Hybrid (Embed Client, Use External Server)

**Approach:**
- Embed gosmee-compatible SSE client in xplat
- Run gosmee server externally (or use hook.pipelinesascode.com)

**Pros:**
- Users get stable URL from public relay or self-hosted
- xplat only needs client code
- Server upgrades don't require xplat updates

**Cons:**
- Depends on external server
- Less control over server behavior

**Effort:** Low-Medium - We already have SSEClient, just need compatibility

---

## Recommendation

**Option B: Cherry-Pick Key Features** with leverage of existing Via SSE patterns.

### Architectural Insight

xplat has two SSE use cases:
1. **UI Updates** (Via) - Browser receives reactive signal updates
2. **Webhook Relay** (gosmee-style) - Go process receives webhook payloads

These are complementary. The Via framework handles #1 well. We need gosmee-style patterns for #2.

### Phase 1: Complete ✅

1. **SSE Client Improvements** (`internal/syncgh/sseclient.go`):
   - ✅ Exponential backoff (1s → 2s → 4s → ... → 60s cap)
   - ✅ Payload saving (`--save-dir`) with replay scripts
   - ✅ Event filtering (`--ignore-event`)
   - ✅ Health endpoint (`--health-port`) for K8s probes

2. **SSE Server** (`internal/syncgh/sseserver.go`):
   - ✅ gosmee-compatible SSE server
   - ✅ EventBroker for fan-out to multiple clients per channel
   - ✅ Webhook signature validation (GitHub, GitLab, Bitbucket, Gitea)
   - ✅ Channel generation (`GET /new`)
   - ✅ SSE events endpoint (`GET /events/{channel}`)
   - ✅ Webhook POST endpoint (`POST /{channel}`)
   - ✅ Health/version endpoint (`GET /health`)

3. **CLI Commands** (`cmd/xplat/cmd/sync_gh.go`):
   - ✅ `xplat sync-gh server` - Run SSE server
   - ✅ `xplat sync-gh sse-client` with new flags

4. **Replay Command** (`internal/syncgh/replay.go`):
   - ✅ List hooks: `xplat sync-gh replay owner/repo --list-hooks`
   - ✅ List deliveries: `xplat sync-gh replay owner/repo HOOK_ID --list-deliveries`
   - ✅ Replay since: `xplat sync-gh replay owner/repo HOOK_ID http://localhost:8763 --since=...`
   - ✅ Continuous mode: `--continuous` flag for watching new deliveries
   - ✅ Save payloads: `--save-dir` flag

### Phase 2: Potential Unification

4. **Consider Via for Webhook UI** - The Via framework could power a webhook debugging UI:
   - Show incoming webhooks in real-time (like gosmee's web UI)
   - Replay webhooks from UI
   - Filter/search webhook history
   - This would unify our SSE usage under Via for all UI needs

5. **Keep Separate SSE for Relay** - gosmee-style SSE relay remains separate:
   - Different protocol (smee.io compatible)
   - Different client (Go process, not browser)
   - Different purpose (relay, not UI)

---

## Feature Comparison

| Feature | xplat (before) | xplat (now) | gosmee |
|---------|----------------|-------------|--------|
| **SSE Server** | ❌ | ✅ Full | ✅ Full |
| **SSE Client** | ✅ Basic | ✅ Full | ✅ Full |
| **Replay** | ❌ | ✅ Full | ✅ Full |
| **Signature Validation** | ✅ GitHub only | ✅ GitHub, GitLab, Bitbucket, Gitea | ✅ Full |
| **Event Filtering** | ❌ | ✅ | ✅ |
| **Payload Saving** | ❌ | ✅ | ✅ |
| **Health Endpoint** | ❌ | ✅ | ✅ |
| **Exponential Backoff** | ❌ (fixed 5s) | ✅ | ✅ |
| **Cache Invalidation** | ✅ | ✅ | ❌ (not their concern) |
| **CF Tunnel Integration** | ✅ | ✅ | ❌ |
| **Task Integration** | ✅ | ✅ | ❌ |

---

## SSE Architecture Comparison

### Via SSE (UI Updates)

```
┌─────────────────────────────────────────────────────────┐
│                  Via SSE Architecture                   │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  Browser                    xplat server                │
│  ┌──────────────┐          ┌──────────────┐            │
│  │ Via JS       │◄────────►│ Via Handler  │            │
│  │ - Signals    │   SSE    │ - c.Signal() │            │
│  │ - Actions    │   HTTP   │ - c.Sync()   │            │
│  └──────────────┘          └──────────────┘            │
│         │                         │                     │
│         │    User clicks "Run"    │                     │
│         │────────────────────────►│                     │
│         │                         │ runTaskWithCallback │
│         │         │◄──────────────│ streams output      │
│         │    SSE: output signal   │                     │
│         │◄────────────────────────│                     │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Key patterns from via_server.go:**
- `c.Signal("")` - Create reactive state
- `c.Action(func() {...})` - Define user actions
- `c.Sync()` - Push SSE update to browser
- `runTaskWithCallback(name, dir, func(line) { output.SetValue(...); c.Sync() })`

### gosmee SSE (Webhook Relay)

```
┌─────────────────────────────────────────────────────────┐
│                gosmee SSE Architecture                  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  GitHub          gosmee server        gosmee client     │
│  ┌───────┐       ┌─────────────┐     ┌─────────────┐   │
│  │Webhook│──────►│ POST /hook  │     │ SSE client  │   │
│  └───────┘       │             │     │             │   │
│                  │ EventBroker │────►│ Forward to  │   │
│                  │ GET /events │ SSE │ localhost   │   │
│                  └─────────────┘     └──────┬──────┘   │
│                                             │          │
│                                      ┌──────▼──────┐   │
│                                      │ Local app   │   │
│                                      │ :8763       │   │
│                                      └─────────────┘   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Key patterns from gosmee:**
- `EventBroker` - Fan-out SSE events to subscribers
- Channel-based routing (`/events/{channel}`)
- smee.io compatible message format
- Reconnection with backoff

### Unified Vision

```
┌─────────────────────────────────────────────────────────┐
│              xplat Unified SSE Architecture             │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  GitHub                xplat server           Browser   │
│  ┌───────┐            ┌─────────────┐       ┌───────┐  │
│  │Webhook│───────────►│ Webhook     │       │ Via   │  │
│  └───────┘            │ Handler     │       │ UI    │  │
│                       │             │       │       │  │
│                       │ ┌─────────┐ │◄─────►│ Tasks │  │
│  SSE Client           │ │EventBus │ │  Via  │ Procs │  │
│  ┌───────────┐◄───────│ │(unified)│ │  SSE  │ Hooks │  │
│  │ Relay     │  SSE   │ └─────────┘ │       └───────┘  │
│  │ Client    │        │             │                   │
│  └───────────┘        └─────────────┘                   │
│        │                                                │
│        ▼ Forward                                        │
│  ┌───────────┐                                          │
│  │ Local     │                                          │
│  │ Handler   │                                          │
│  └───────────┘                                          │
│                                                         │
└─────────────────────────────────────────────────────────┘

Key: Via SSE for UI, gosmee-style SSE for relay
     Both can share an EventBus for webhook debugging UI
```

---

## Code Mapping

### Replay Command (to add)

```go
// cmd/xplat/cmd/sync_gh.go

var syncGHReplayCmd = &cobra.Command{
    Use:   "replay <owner/repo> [hook-id] [target-url]",
    Short: "Replay past webhook deliveries from GitHub",
    Long: `Fetch and replay webhook deliveries from GitHub API.

This is useful when:
  - You missed webhooks while your relay was down
  - Testing webhook handlers with real payloads
  - Debugging webhook processing issues

Examples:
  # List hooks on a repo
  xplat sync-gh replay --list-hooks owner/repo

  # List recent deliveries for a hook
  xplat sync-gh replay --list-deliveries owner/repo 12345

  # Replay all deliveries since a timestamp
  xplat sync-gh replay owner/repo 12345 http://localhost:8763 --since=2024-01-01T00:00:00`,
}
```

### Improved SSEClient (to enhance)

```go
// internal/syncgh/sseclient.go

type SSEClientConfig struct {
    ServerURL     string
    TargetURL     string
    SaveDir       string        // NEW: Save payloads to disk
    IgnoreEvents  []string      // NEW: Skip these event types
    HealthPort    int           // NEW: Expose health endpoint
    OnEvent       func(...)
}

// Use exponential backoff from cenkalti/backoff
func (c *SSEClient) connectWithBackoff(ctx context.Context) error {
    b := backoff.NewExponentialBackOff()
    b.MaxElapsedTime = 0 // Retry forever
    return backoff.Retry(func() error {
        return c.connect(ctx)
    }, b)
}
```

### Server SSE Broadcast (to add)

```go
// internal/syncgh/webhook.go

type WebhookServer struct {
    handler     *githubevents.EventHandler
    port        string
    config      WebhookConfig
    eventBroker *EventBroker  // NEW: For SSE broadcast
}

// NEW: SSE endpoint
func (s *WebhookServer) handleSSE(w http.ResponseWriter, r *http.Request) {
    channel := chi.URLParam(r, "channel")
    subscriber := s.eventBroker.Subscribe(channel)
    defer s.eventBroker.Unsubscribe(channel, subscriber)

    // Stream events to client...
}
```

---

## Dependencies to Add

```go
// For exponential backoff
"gopkg.in/cenkalti/backoff.v1"

// For SSE server (if not using raw implementation)
"github.com/r3labs/sse/v2"
```

---

## Migration Path

### For Users Currently Using gosmee

If users are already using gosmee:
1. They can continue using `gosmee server` + `xplat sync-gh sse-client`
2. Our SSE client is compatible with gosmee's message format
3. Or switch to `xplat sync-gh server` for full xplat integration

### For New Users

Recommended setup by use case:

1. **Simplest** (polling, 5-min delay):
   ```bash
   xplat sync-gh poll --invalidate
   ```

2. **Real-time dev** (URL changes on restart):
   ```bash
   xplat sync-gh relay
   ```

3. **Real-time with xplat server** (self-hosted, stable URL):
   ```bash
   # Terminal 1: Start SSE server
   xplat sync-gh server --port=3333

   # Terminal 2: Start tunnel for public URL
   xplat sync-cf tunnel --port=3333

   # Terminal 3: Connect client with cache invalidation
   xplat sync-gh sse-client https://<tunnel-url>/<channel> --invalidate

   # Configure GitHub webhook to: https://<tunnel-url>/<channel>
   ```

4. **Production** (external gosmee or xplat server behind load balancer):
   ```bash
   xplat sync-gh sse-client https://webhook.example.com/channel --invalidate
   ```

---

## Process Compose Service Example

Add syncgh to process-compose for automatic webhook relay:

```yaml
# process-compose.yaml
version: "0.5"

processes:
  # SSE Server - receives GitHub webhooks
  syncgh-server:
    command: xplat sync-gh server --port=3333
    namespace: sync
    readiness_probe:
      http_get:
        scheme: http
        host: 127.0.0.1
        port: 3333
        path: /health
      initial_delay_seconds: 2
      period_seconds: 10

  # Cloudflare tunnel - exposes server publicly
  syncgh-tunnel:
    command: xplat sync-cf tunnel --port=3333
    namespace: sync
    depends_on:
      syncgh-server:
        condition: process_healthy

  # SSE Client - forwards to local webhook handler with cache invalidation
  # Use this if you're connecting to an external gosmee/SSE server
  syncgh-client:
    command: xplat sync-gh sse-client ${WEBHOOK_URL} --invalidate --health-port=8081
    namespace: sync
    disabled: true  # Enable when using external server
    environment:
      - WEBHOOK_URL=https://hook.pipelinesascode.com/your-channel
    readiness_probe:
      http_get:
        scheme: http
        host: 127.0.0.1
        port: 8081
        path: /health
      initial_delay_seconds: 2
      period_seconds: 10

  # Polling fallback - use when webhooks aren't available
  syncgh-poll:
    command: xplat sync-gh poll --interval=5m --invalidate
    namespace: sync
    disabled: true  # Enable as fallback
```

Usage:
```bash
# Start with server + tunnel (development)
xplat process -p syncgh-server,syncgh-tunnel

# Start with external SSE server (production)
WEBHOOK_URL=https://webhook.example.com/channel xplat process -p syncgh-client

# Start with polling fallback
xplat process -p syncgh-poll
```

---

## References

- [gosmee GitHub](https://github.com/chmouel/gosmee)
- [hook.pipelinesascode.com](https://hook.pipelinesascode.com) - Public gosmee relay
- [smee.io](https://smee.io) - Original Node.js webhook relay
- [Cloudflare Tunnels](https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/)
