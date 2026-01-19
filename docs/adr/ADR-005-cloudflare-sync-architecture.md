# ADR-005: Cloudflare Sync Architecture

## Status

**Active** - Decision made, implementation in progress

## Decision

**Option 1: Worker-Centric** - Keep the CF Worker as the primary event receiver, add a local receiver (`xplat sync-cf receive`) to complete the round-trip.

## Primary Use Case: Round-Trip Validation

The Cloudflare sync system enables **round-trip validation** for the xplat setup wizards:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Round-Trip Validation Flow                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. User runs setup wizard (internal/env/web/):                            │
│     - Step 1: Configure API token                                          │
│     - Step 2: Select account                                               │
│     - Step 3: Choose domain                                                │
│     - Step 4: Create/select Pages project                                  │
│     - Step 5: Attach custom domain                                         │
│     - Step 6: Enable event notifications (NEW)                             │
│                                                                             │
│  2. User deploys site:                                                     │
│     - xplat env deploy → builds Hugo → uploads to Pages                    │
│                                                                             │
│  3. Cloudflare sends event to Worker:                                      │
│     - Pages deploy hook fires                                              │
│     - Worker normalizes and forwards to SYNC_ENDPOINT                      │
│                                                                             │
│  4. Local receiver validates:                                              │
│     - "Deploy succeeded for project X"                                     │
│     - Can trigger cache invalidation, notifications, etc.                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Why this matters:**
- Setup wizards modify CF state (create projects, add domains, deploy)
- Round-trip validation confirms those operations succeeded
- Real-time feedback is important for developer experience

## Context

We currently have **two overlapping implementations** for receiving Cloudflare events:

| Component | Location | Runs Where | Purpose |
|-----------|----------|------------|---------|
| **CF Worker** | `workers/sync-cf/` | Cloudflare Edge | Receives CF events, forwards to sync endpoint |
| **synccf package** | `internal/synccf/` | Local machine | Receives webhooks, handles events |

### Why the Worker Was Created

The Cloudflare Worker was created because:

1. **Public endpoint needed** - Cloudflare webhooks (Pages deploy hooks, Notifications) require a publicly accessible HTTPS URL
2. **Always-on** - Workers run 24/7 on Cloudflare's edge, no local server needed
3. **Event aggregation** - Single endpoint to receive events from multiple CF services:
   - Pages deploy hooks
   - Notification webhooks (alerts)
   - Logpush HTTP destinations
4. **Forwarding** - Sends normalized events to your local sync endpoint (via tunnel)

### Why synccf Package Exists

The `internal/synccf/` package was created because:

1. **Local development** - Run webhook handlers locally for testing
2. **Direct integration** - When you CAN expose a local port (via cloudflared tunnel)
3. **Event handling** - Register callbacks for different event types
4. **Audit polling** - Poll CF audit logs (alternative to webhooks)

### The Problem

**These two approaches overlap but don't integrate:**

```
Current State (Broken):

┌─────────────────┐     ┌──────────────────┐
│ Cloudflare      │────▶│ CF Worker        │
│ (webhooks)      │     │ workers/sync-cf  │
└─────────────────┘     └────────┬─────────┘
                                 │ forwards to SYNC_ENDPOINT
                                 ▼
                        ┌──────────────────┐
                        │ ??? Nothing      │  ← No receiver!
                        │ listens here     │
                        └──────────────────┘

Meanwhile, separately:

┌─────────────────┐     ┌──────────────────┐
│ Cloudflare      │────▶│ synccf package   │  ← Expects direct CF format
│ (via tunnel)    │     │ internal/synccf  │
└─────────────────┘     └──────────────────┘
```

**Key issues:**

1. Worker forwards events, but nothing receives them
2. synccf expects direct Cloudflare webhook format, not Worker's normalized format
3. No connection to Task cache invalidation (unlike syncgh)
4. Two codebases to maintain for similar functionality

---

## Analysis: When Would You Use Each?

### Scenario A: Production (Worker)

```
CF Services → CF Worker → Your Tunnel → Local Handler → Cache Invalidation
```

**Use when:**
- You need 24/7 event reception
- Your local machine isn't always on
- You want event aggregation at the edge
- You want to buffer/batch events

**Problem:** No local handler exists to receive forwarded events

### Scenario B: Development (Direct)

```
CF Services → Tunnel → synccf Handler → Cache Invalidation
```

**Use when:**
- Local development/testing
- You're running xplat continuously
- You want simpler setup (no Worker deployment)

**Problem:** Requires public tunnel URL, synccf not wired to cache invalidation

### Scenario C: Polling (No Webhooks)

```
Poller → CF Audit API → Cache Invalidation
```

**Use when:**
- You can't/don't want to set up webhooks
- Lower frequency is acceptable (polling interval)
- Simpler setup, no public URL needed

**Problem:** synccf has AuditPoller but it's not fully implemented

---

## Comparison with syncgh

The `syncgh` package (GitHub sync) is more mature:

| Feature | syncgh | synccf |
|---------|--------|--------|
| Polling | ✅ StatefulPoller with persistence | ⚠️ AuditPoller (incomplete) |
| Webhooks | ✅ WebhookServer with handlers | ✅ WebhookHandler |
| SSE/gosmee | ✅ Full gosmee integration | ❌ None |
| Cache invalidation | ✅ TaskCacheInvalidator callback | ❌ Not implemented |
| State persistence | ✅ `~/.xplat/cache/syncgh-poll-state.json` | ❌ None |
| Edge worker | ❌ Not needed (GitHub has smee.io) | ✅ workers/sync-cf |

**Key insight:** GitHub has smee.io for webhook relay, so we use gosmee. Cloudflare doesn't have an equivalent, so we built a Worker.

---

## Options

### Option 1: Worker-Centric (Recommended)

**Keep the Worker as the primary event receiver, add a local receiver.**

```
┌─────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│ Cloudflare      │────▶│ CF Worker        │────▶│ xplat sync-cf    │
│ (all webhooks)  │     │ workers/sync-cf  │     │ receive          │
└─────────────────┘     └──────────────────┘     └────────┬─────────┘
                                                          │
                                                          ▼
                                                 ┌──────────────────┐
                                                 │ Cache            │
                                                 │ Invalidation     │
                                                 └──────────────────┘
```

**Changes needed:**
1. Add `xplat sync-cf receive` command - HTTP server that receives Worker's forwarded events
2. Wire up cache invalidation callback (like syncgh)
3. Deprecate direct webhook handlers in synccf (or keep for local dev only)

**Pros:**
- Worker handles all CF complexity at the edge
- Local receiver is simple (just parse Worker's normalized format)
- Works even when local machine is offline (Worker buffers? Or just drops)

**Cons:**
- Two things to deploy/maintain (Worker + local receiver)
- Extra hop (CF → Worker → Local)

### Option 2: Direct-Only (Simplify)

**Remove the Worker, use cloudflared tunnel directly.**

```
┌─────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│ Cloudflare      │────▶│ cloudflared      │────▶│ synccf handlers  │
│ (all webhooks)  │     │ tunnel           │     │ internal/synccf  │
└─────────────────┘     └──────────────────┘     └────────┬─────────┘
                                                          │
                                                          ▼
                                                 ┌──────────────────┐
                                                 │ Cache            │
                                                 │ Invalidation     │
                                                 └──────────────────┘
```

**Changes needed:**
1. Add `xplat sync-cf tunnel` command that starts cloudflared + webhook server
2. Wire up cache invalidation callback
3. Delete `workers/sync-cf/`

**Pros:**
- Simpler architecture (no Worker)
- One less thing to deploy
- Direct event handling

**Cons:**
- Requires cloudflared binary
- Requires always-running local process
- Tunnel URL changes on restart (need to update CF webhook configs)

**Tunnel URL Auto-Discovery Enhancement:**

The tunnel URL issue can be solved using the SignTools pattern (see ADR-007). When cloudflared starts with `-metrics localhost:51881`, we can scrape the metrics endpoint to auto-discover the public URL:

```go
// Auto-discover tunnel URL from cloudflared metrics
var publicUrlRegex = regexp.MustCompile(
    `cloudflared_tunnel_user_hostnames_counts{userHostname="(.+)"}`)

func GetTunnelURL(metricsHost string) (string, error) {
    resp, _ := http.Get(fmt.Sprintf("http://%s/metrics", metricsHost))
    data, _ := io.ReadAll(resp.Body)
    if matches := publicUrlRegex.FindStringSubmatch(string(data)); len(matches) > 0 {
        return matches[1], nil  // e.g., "https://xxx.trycloudflare.com"
    }
    return "", ErrTunnelNotFound
}
```

This eliminates manual URL configuration - xplat can auto-detect the tunnel URL and programmatically register it with Cloudflare webhook endpoints.

### Option 3: Polling-Only (Simplest)

**Remove both Worker and webhooks, just poll.**

```
┌──────────────────┐     ┌──────────────────┐
│ CF Audit API     │◀────│ synccf Poller    │
│ (pull)           │     │                  │
└──────────────────┘     └────────┬─────────┘
                                  │
                                  ▼
                         ┌──────────────────┐
                         │ Cache            │
                         │ Invalidation     │
                         └──────────────────┘
```

**Changes needed:**
1. Complete AuditPoller implementation
2. Add state persistence (like syncgh)
3. Wire up cache invalidation
4. Delete `workers/sync-cf/`

**Pros:**
- Simplest architecture
- No public URLs needed
- No external dependencies (cloudflared, Worker)
- Works behind firewalls

**Cons:**
- Polling delay (not real-time)
- API rate limits
- May miss events if polling interval too long

### Option 4: Hybrid (Most Flexible)

**Support all three modes, let user choose.**

```yaml
# xplat.yaml
sync:
  cloudflare:
    mode: worker  # or "tunnel" or "poll"
    worker_url: https://my-worker.workers.dev
    poll_interval: 5m
```

**Pros:**
- Maximum flexibility
- User chooses based on their constraints

**Cons:**
- More code to maintain
- More documentation needed
- User has to understand tradeoffs

---

## Questions to Answer

1. **What's the primary use case?**
   - Real-time cache invalidation when CF Pages deploys?
   - Audit logging for compliance?
   - Monitoring CF health events?

2. **What's the deployment environment?**
   - Always-on server (can run tunnel)?
   - Intermittent developer machine?
   - CI only?

3. **Is real-time needed?**
   - If yes: Worker or Tunnel approach
   - If no: Polling is simpler

4. **Do we need the Worker at all?**
   - If we're always running xplat locally, tunnel may be enough
   - Worker is useful if local machine isn't always on

---

## Decision Rationale

**Why Option 1 (Worker-Centric):**

1. **Worker already exists** - `workers/sync-cf/` is built, just needs a receiver
2. **Edge does the work** - Worker normalizes events from multiple CF sources
3. **Supports round-trip validation** - See events from things you just configured
4. **Real-time feedback** - Instant notification when deploys complete
5. **No local tunnel required** - CF sends to Worker → Worker sends to your tunnel

**Why NOT the other options:**

- **Option 2 (Direct)**: Requires cloudflared binary and stable tunnel URL
- **Option 3 (Polling)**: Not real-time, polling delay breaks validation UX
- **Option 4 (Hybrid)**: Over-engineering for now, can add later if needed

## Immediate Action Items

1. ✅ **Add `xplat sync-cf receive` command** - HTTP server that receives Worker events
2. ✅ **Wire up event callbacks** - OnPagesDeploy, OnAlert, OnLogpush, OnAny
3. ✅ **Add state persistence** - `~/.xplat/cache/synccf-receive-state.json`
4. ✅ **Add Task cache invalidation** - `--invalidate` flag clears `.task/remote/` on Pages deploy
5. ✅ **Add Step 6 to wizard** - Event Notifications setup in web UI
6. ✅ **Wire .env config to CLI** - `sync-cf receive` and `sync-cf tunnel` use receiver port from .env
7. ⬜ **Document the flow** - How to deploy Worker + configure SYNC_ENDPOINT
8. ⬜ **Add Worker deployment API** - Deploy Worker via Cloudflare API (no wrangler needed)

**Future consideration:**

- Add polling as fallback when Worker/tunnel isn't available
- Consider hybrid mode if users need different approaches

---

## Implementation Plan

### Phase 1: Wire Up Worker → Local ✅ COMPLETE

1. [x] Add `xplat sync-cf receive` command
2. [x] Parse Worker's Event format (already normalized)
3. [x] Add `OnEvent` callback registration (OnPagesDeploy, OnAlert, OnLogpush, OnAny)
4. [x] Add Task cache invalidation callback (`synccf.TaskCacheInvalidator`)

### Phase 2: State & Persistence ✅ COMPLETE

1. [x] Add `~/.xplat/cache/synccf-receive-state.json`
2. [x] Track processed event IDs to avoid duplicates (by event key)
3. [x] Add `xplat sync-cf receive-state` to show state

### Phase 3: Web UI Integration ✅ COMPLETE

1. [x] Add Step 6 (Event Notifications) to Cloudflare wizard
2. [x] Add environment variables: `CLOUDFLARE_WORKER_NAME`, `CLOUDFLARE_SYNC_ENDPOINT`, `CLOUDFLARE_RECEIVER_PORT`
3. [x] Wire .env config to CLI commands (receiver and tunnel use port from .env)

### Phase 4: Process Compose Integration

1. [x] Add synccf-receive to process-compose example
2. [ ] Document Worker deployment + local receiver setup

### Phase 5: Polish

1. [x] Add health check endpoint (`/health`)
2. [x] Add status endpoint (`/status` shows event count, last event time)
3. [ ] Add metrics/observability
4. [ ] Consider polling fallback

### Phase 6: Worker Deployment (Future)

Worker deployment via Cloudflare API (no wrangler) requires:
1. [ ] Add Worker API endpoints to `internal/env/constants.go`
2. [ ] Create multipart form upload for Worker script
3. [ ] Handle Worker bindings (SYNC_ENDPOINT environment variable)
4. [ ] Wire deployment to Step 6 UI "Deploy Worker" button

**Note:** Currently using wrangler CLI for Worker deployment (`xplat sync-cf worker deploy`).
The Cloudflare Workers API uses multipart form uploads with metadata, which is more complex
than the simple REST calls used for Pages. Consider keeping wrangler dependency for now.

---

## References

- Worker source: `workers/sync-cf/`
- synccf package: `internal/synccf/`
- syncgh package: `internal/syncgh/` (reference implementation)
- ADR-004: gosmee integration (similar pattern for GitHub)
- ADR-007: Cloudflare tunnel patterns (SignTools metrics scraping for URL auto-discovery)
