# ADR-005: Cloudflare Sync Architecture

## Status

**Proposed** - Needs analysis and decision

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

## Recommendation

**Start with Option 1 (Worker-Centric) because:**

1. Worker is already built and deployed
2. Matches the "edge does the work" philosophy
3. Can evolve to Option 4 later if needed

**Immediate action items:**

1. Add `xplat sync-cf receive` - receives Worker's forwarded events
2. Add cache invalidation callback (mirror syncgh pattern)
3. Add state persistence for tracking processed events
4. Document the Worker → Local flow

**Future consideration:**

- Add polling as fallback when Worker/tunnel isn't available
- Consider Option 4 (hybrid) if users need different modes

---

## Implementation Plan

### Phase 1: Wire Up Worker → Local

1. [ ] Add `xplat sync-cf receive` command
2. [ ] Parse Worker's Event format (already normalized)
3. [ ] Add `OnEvent` callback registration
4. [ ] Add Task cache invalidation callback

### Phase 2: State & Persistence

1. [ ] Add `~/.xplat/cache/synccf-state.json`
2. [ ] Track processed event IDs to avoid duplicates
3. [ ] Add `xplat sync-cf status` to show state

### Phase 3: Process Compose Integration

1. [ ] Add synccf-receive to process-compose example
2. [ ] Document Worker deployment + local receiver setup

### Phase 4: Polish

1. [ ] Add metrics/observability
2. [ ] Add health check endpoint
3. [ ] Consider polling fallback

---

## References

- Worker source: `workers/sync-cf/`
- synccf package: `internal/synccf/`
- syncgh package: `internal/syncgh/` (reference implementation)
- ADR-004: gosmee integration (similar pattern for GitHub)
