# ADR-011: Datastar Skills for Claude Code

## Status

**Adopt** - Install for Claude Code expertise in NATS and Datastar

## Context

[cbeauhilton/datastar-skills](https://github.com/cbeauhilton/datastar-skills) provides Claude Code skills for two key technologies:

1. **Datastar** - Hypermedia framework for reactive web apps
2. **NATS JetStream** - Distributed messaging with persistence

These skills make Claude an expert in these technologies by providing comprehensive documentation, patterns, and best practices directly in context.

**Source**: Cloned to `.src/datastar-skills/`

---

## What Are Claude Code Skills?

Skills are markdown files that Claude Code loads into context when invoked. They provide:

- Domain knowledge (concepts, APIs, configuration)
- Best practices and patterns
- Anti-patterns to avoid
- Code examples in relevant languages

When you have a skill installed, Claude "knows" the technology deeply.

---

## Skill 1: Datastar

### What is Datastar?

Lightweight (~11KB) hypermedia framework for reactive web apps. **Backend drives frontend** via SSE.

Key philosophy: "The Tao of Datastar"
- Backend is source of truth
- DOM patching over fine-grained updates
- Restrained signal usage
- No optimistic updates

### Core Capabilities

| Feature | Description |
|---------|-------------|
| SSE Streaming | Server pushes HTML fragments + signal updates |
| DOM Morphing | Only modified parts update, state preserved |
| Signals | Reactive state (`data-signals`) |
| Actions | Backend calls (`@get`, `@post`, `@put`, `@delete`) |
| Binding | Two-way input binding (`data-bind`) |

### Example Pattern

```html
<!-- Form with backend-driven validation -->
<form data-signals="{email: '', password: ''}">
  <input type="email" data-bind:value="email">
  <input type="password" data-bind:value="password">
  <button data-on:click="@post('/login')"
          data-indicator="#loading">
    Login
  </button>
  <span id="loading" style="display:none">Logging in...</span>
</form>
```

```go
// Go backend
func loginHandler(w http.ResponseWriter, r *http.Request) {
    signals := datastar.ReadSignals(r)

    sse := datastar.NewSSE(w)
    if valid {
        sse.Redirect("/dashboard")
    } else {
        sse.PatchElements("#error", "<div>Invalid credentials</div>")
    }
}
```

### Anti-Patterns to Avoid

1. **Optimistic updates** - Don't show success before backend confirms
2. **Frontend state management** - Keep business state on backend
3. **Custom history management** - Use standard navigation
4. **Trusting frontend cache** - Always fetch current state

### Skill Contents

```
datastar/
├── skills/datastar.md      # Main skill (philosophy, quick start)
├── patterns/
│   ├── tao.md              # The Datastar Way
│   └── howtos.md           # Common implementations
└── reference/              # Attribute reference
```

---

## Skill 2: NATS JetStream

### What is JetStream?

NATS's built-in persistence layer. Core mental model:

> **Streams store messages. Consumers read them.**

| Concept | Description |
|---------|-------------|
| Streams | Append-only logs capturing messages by subject |
| Consumers | Cursors/views that track position and replay |
| Acknowledgment | Critical for delivery guarantees |

### When to Use

**Use JetStream:**
- Temporal decoupling (async producers/consumers)
- Message replay needed
- At-least-once or exactly-once delivery
- Work queues

**Use Core NATS:**
- Tight request-reply
- Ephemeral/low-TTL data
- Control plane messages

### Key Patterns Covered

#### Work Queues
```go
stream, _ := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
    Name:      "TASKS",
    Subjects:  []string{"tasks.>"},
    Retention: jetstream.WorkQueuePolicy,  // Delete after ack
})
```

#### Exactly-Once Delivery
```go
// Deduplication via message ID
js.Publish(ctx, "orders.created", data,
    jetstream.WithMsgID(fmt.Sprintf("order-%s-%d", orderID, version)))
```

#### Event Sourcing
```go
// Append-only event log
subject := fmt.Sprintf("orders.%s.%s", orderID, eventType)
js.Publish(ctx, subject, eventData,
    jetstream.WithExpectLastSubjectSequence(lastSeq))  // Optimistic lock
```

### Consumer Types

| Type | Persistence | Use Case |
|------|-------------|----------|
| Durable | Named, survives disconnect | Production workloads |
| Ephemeral | Auto-deleted | Temporary processing |
| Ordered | Ephemeral + flow control | Simple consumption |

### Acknowledgment Patterns

| Ack Type | Behavior |
|----------|----------|
| `Ack()` | Success, remove from pending |
| `Nak()` | Failed, redeliver immediately |
| `InProgress()` | Extend processing deadline |
| `Term()` | Stop redelivery (poison message) |
| `DoubleAck()` | Wait for server confirmation |

### Skill Contents

```
nats-jetstream/
├── skills/nats-jetstream.md  # Main skill (concepts, quick start)
├── concepts/
│   ├── streams.md            # Stream configuration
│   ├── consumers.md          # Consumer types
│   ├── subjects.md           # Subject hierarchy
│   └── acknowledgment.md     # Ack patterns
├── patterns/
│   ├── work-queues.md        # Competing consumers
│   ├── fan-out.md            # Pub/sub patterns
│   ├── exactly-once.md       # Deduplication
│   └── event-sourcing.md     # DCB patterns
├── reference/
│   ├── stream-config.md      # All options
│   ├── consumer-config.md    # All options
│   └── cli.md                # nats CLI
└── sdks/
    └── go.md                 # Go SDK patterns
```

---

## xplat Relevance

### High Value for xplat Development

| Technology | xplat Use Case |
|------------|----------------|
| **NATS JetStream** | syncgh event streaming, message persistence |
| **Datastar** | Task UI dashboard, real-time process status |

### Why These Skills Together?

Datastar + NATS JetStream = **Real-time reactive apps**

```
User Action → Datastar → Backend → NATS JetStream
                                         ↓
                                   Event Stored
                                         ↓
                              Other Consumers ← Fan-out
                                         ↓
                              SSE Updates → Datastar → UI
```

xplat already uses NATS (via embeddednats from toolbelt). Adding Datastar enables:
- Real-time Task UI updates
- Process status streaming
- Collaborative debugging views

---

## Installation

### For Claude Code CLI

```bash
# Add to Claude Code config
claude skill install cbeauhilton/datastar-skills/datastar
claude skill install cbeauhilton/datastar-skills/nats-jetstream
```

### For Claude Code IDE Extension

Add to `.claude/settings.json`:
```json
{
  "skills": [
    "cbeauhilton/datastar-skills/datastar",
    "cbeauhilton/datastar-skills/nats-jetstream"
  ]
}
```

---

## Benefits

### Before Skills

```
User: "How do I set up a JetStream work queue?"
Claude: [Generic answer, may miss nuances]
```

### After Skills

```
User: "How do I set up a JetStream work queue?"
Claude: [Detailed answer with]:
- Correct WorkQueuePolicy retention
- Pull consumer configuration
- Proper acknowledgment handling
- Common gotchas (non-overlapping consumers)
- Go code example
```

The skill provides **authoritative, up-to-date knowledge** directly in context.

---

## Skill Quality

### Datastar Skill

- **Completeness**: Covers philosophy, attributes, patterns, SSE
- **Examples**: Go backend integration included
- **Anti-patterns**: Well-documented "what not to do"

### NATS JetStream Skill

- **Depth**: Concepts, patterns, reference, SDK examples
- **Patterns**: Event sourcing, exactly-once, DCB documented
- **Practical**: Go code throughout, CLI reference included

---

## MCP vs Skills: When to Use Which?

You mentioned searching for an MCP. Here's the key distinction:

### Claude Code Skills (This Approach)

**What they do**: Inject documentation/patterns into Claude's context

| Aspect | Skills |
|--------|--------|
| **Purpose** | Make Claude an expert in a technology |
| **How** | Markdown files loaded into context |
| **Runtime** | No runtime, just knowledge |
| **Actions** | Claude writes code, YOU run it |
| **Updates** | Manual (update skill files) |

**Best for:**
- Learning technologies (NATS, Datastar)
- Code generation with correct patterns
- Architecture guidance
- Avoiding anti-patterns

### MCP (Model Context Protocol)

**What they do**: Give Claude tools to interact with external systems

| Aspect | MCP |
|--------|-----|
| **Purpose** | Let Claude take actions |
| **How** | Server exposes tools Claude can call |
| **Runtime** | Running server (local or remote) |
| **Actions** | Claude directly runs commands/queries |
| **Updates** | Live (reads from actual systems) |

**Best for:**
- Running commands (`xplat mcp` exposes Taskfile tasks)
- Querying databases/APIs
- File operations
- System interactions

### Comparison Table

| Need | Use Skills | Use MCP |
|------|------------|---------|
| "How do I configure JetStream?" | ✅ | ❌ |
| "Create a stream on my NATS server" | ❌ | ✅ |
| "Best practices for Datastar forms?" | ✅ | ❌ |
| "Run `task build` for me" | ❌ | ✅ |
| "Generate a Datastar component" | ✅ | ❌ |
| "Show me the task list" | ❌ | ✅ |

### The Right Answer: Both

**Skills + MCP are complementary:**

```
┌─────────────────────────────────────────────────────────────┐
│                    Claude Code Session                       │
├─────────────────────────────────────────────────────────────┤
│  Skills (Knowledge)           │  MCP (Actions)              │
│  ────────────────────         │  ────────────────           │
│  • NATS JetStream patterns    │  • xplat task run           │
│  • Datastar philosophy        │  • Query NATS streams       │
│  • Event sourcing howtos      │  • File operations          │
│  • Go SDK examples            │  • Git operations           │
│                               │                             │
│  "I know HOW to do it"        │  "I CAN do it for you"      │
└─────────────────────────────────────────────────────────────┘
```

### xplat Strategy

| Technology | Skill | MCP |
|------------|-------|-----|
| **NATS** | ✅ nats-jetstream skill | Future: nats MCP for stream ops |
| **Datastar** | ✅ datastar skill | N/A (frontend framework) |
| **Taskfile** | Future: xplat-taskfile skill | ✅ `xplat mcp serve` |
| **Process Compose** | Future skill | ✅ `xplat mcp serve` |

### Bottom Line

- **Skills**: Claude knows the patterns, YOU execute
- **MCP**: Claude can execute for you
- **For NATS/Datastar**: Skills are the right choice (knowledge transfer)
- **For xplat operations**: MCP is the right choice (Claude runs tasks)

You already have `xplat mcp serve` (ADR-008). Adding these skills gives Claude the domain expertise to write better code.

---

## Recommendations

### Immediate Actions

1. **Install both skills** for Claude Code sessions
2. **Use for xplat development** when building:
   - Task UI (Datastar)
   - Event streaming (JetStream)
   - Real-time features (Both)

### Future Enhancements

Consider creating xplat-specific skills:
- `xplat-taskfile` - Taskfile patterns and conventions
- `xplat-process-compose` - Process orchestration patterns

---

## References

- Repository: https://github.com/cbeauhilton/datastar-skills
- Datastar: https://data-star.dev
- NATS: https://docs.nats.io
- NATS by Example: https://natsbyexample.com
- Related: ADR-009 (toolbelt - has embeddednats)
