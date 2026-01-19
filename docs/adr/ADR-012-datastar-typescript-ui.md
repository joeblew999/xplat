# ADR-012: Datastar TypeScript SDK for User UI Development

## Status

**Proposed** - Adding Datastar TypeScript support to xplat toolchain

https://github.com/starfederation/datastar-typescript

### Cloudflare Workers Compatibility

**Assessed as VIABLE** - The SDK's `/web` export uses only Web Standard APIs:
- `ReadableStream`, `ReadableStreamDefaultController`, `TextEncoder`
- `Response`, `Request`, `URL`, `URLSearchParams`

The `stream()` method returns a `Response` object directly, which is exactly what CF Workers expects.

**Caveats:**
- Official SDK doesn't explicitly document CF Workers support
- [m0hill/datastar-demo](https://github.com/m0hill/datastar-demo) uses custom SSE, not the official SDK
- Recommend testing with a minimal CF Worker before committing

**Source analysis:** `.src/datastar-typescript/src/web/serverSentEventGenerator.ts`

## Context

xplat users need to build reactive UIs that:

1. **Run locally AND on Cloudflare Workers** - Same code, both environments
2. **Are easy to generate with AI** - TypeScript has better AI tooling support
3. **Require minimal JavaScript expertise** - Hypermedia-driven, not SPA complexity
4. **Integrate with xplat services** - SSE streaming, NATS events

### The Problem

Building modern UIs typically requires:
- Heavy frameworks (React, Vue, Svelte) with complex build tooling
- Deep JavaScript/TypeScript expertise
- Separate codebases for local dev vs edge deployment
- Manual SSE/WebSocket implementation for real-time features

### Why TypeScript for User UIs?

| Factor | Benefit for xplat Users |
|--------|------------------------|
| AI code generation | Claude/GPT generate TypeScript better than any other language |
| CF Worker native | No TinyGo/WASM compilation - just deploy |
| Hot reload | Instant feedback during development |
| Type safety | Catch errors before runtime |
| Bun ecosystem | Fast runtime, built-in bundler, TypeScript-native |

**Key insight**: Users can describe what they want, AI generates the TypeScript, xplat runs it locally and on edge.

---

## Decision

**xplat will provide Datastar TypeScript scaffolding and tooling so users can easily build reactive UIs that run locally and on Cloudflare Workers.**

### What xplat Provides

| Component | Purpose |
|-----------|---------|
| `xplat ui init` | Scaffold a Datastar TypeScript project |
| `xplat ui dev` | Run local dev server (Bun) |
| `xplat ui build` | Bundle for production |
| `xplat ui deploy` | Deploy to Cloudflare Workers |
| Templates | Starter templates for common UI patterns |
| Skills | Claude Code skills for AI-assisted development |

### Why Datastar?

| Alternative | Why Not for xplat Users |
|-------------|------------------------|
| React/Vue/Svelte | Heavy frameworks, steep learning curve |
| HTMX | Similar, but Datastar has better signals + SSE built-in |
| Alpine.js | Client-only, no SSE streaming |
| Raw HTML/JS | Too low-level, no reactive primitives |

Datastar is ideal for xplat users:
- **~11KB** client library
- **Backend-driven** - Server pushes HTML via SSE (matches xplat's Go services)
- **Declarative** - `data-*` attributes, minimal JS knowledge needed
- **AI-friendly** - Simple patterns AI can generate correctly

---

## Architecture: User's Perspective

```
┌─────────────────────────────────────────────────────────────────┐
│                   User's xplat Project                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Taskfile.yml          # User's tasks                          │
│  process-compose.yaml  # User's services                       │
│                                                                 │
│  services/             # Go/Rust backend services              │
│  └── api/                                                       │
│                                                                 │
│  ui/                   # Datastar TypeScript (xplat scaffolded)│
│  ├── src/                                                       │
│  │   ├── index.ts      # Hono app                              │
│  │   ├── routes/       # UI routes                             │
│  │   └── components/   # Reusable fragments                    │
│  ├── package.json                                               │
│  └── wrangler.toml     # CF Worker config                      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

User workflow:
1. xplat ui init              → Scaffolds ui/ directory
2. Describe UI to Claude      → AI generates TypeScript
3. task ui:dev                → Run locally
4. task start                 → Run with all services
5. xplat ui deploy           → Deploy to CF Workers
```

---

## The AI-First Workflow

This is the key differentiator: **users describe UIs in natural language, AI generates code, xplat runs it.**

### Step 1: User Describes Intent

```
User: "Create a dashboard that shows my task list with real-time updates.
       Each task should have a checkbox and delete button."
```

### Step 2: AI Generates Datastar TypeScript

Claude (with Datastar skill from ADR-011) generates:

```typescript
// src/routes/dashboard.ts
import { Hono } from 'hono';
import { ServerSentEventGenerator } from '@starfederation/datastar-sdk/web';

const app = new Hono();

app.get('/', (c) => c.html(`
  <div id="tasks"
       data-on-load="@get('/api/tasks/stream')"
       class="space-y-2">
    Loading...
  </div>
`));

app.get('/api/tasks/stream', async (c) => {
  return ServerSentEventGenerator.stream(c.req.raw, c.res, async (stream) => {
    const tasks = await fetchTasks();

    stream.patchElements(`
      <div id="tasks" class="space-y-2">
        ${tasks.map(t => `
          <div id="task-${t.id}" class="flex items-center gap-2 p-2 border rounded">
            <input type="checkbox"
                   ${t.done ? 'checked' : ''}
                   data-on:change="@post('/api/tasks/${t.id}/toggle')">
            <span class="${t.done ? 'line-through' : ''}">${t.name}</span>
            <button data-on:click="@delete('/api/tasks/${t.id}')"
                    class="ml-auto text-red-500">
              Delete
            </button>
          </div>
        `).join('')}
      </div>
    `);
  });
});

export default app;
```

### Step 3: User Runs Locally

```bash
task ui:dev  # Starts Bun dev server with hot reload
```

### Step 4: User Deploys to Edge

```bash
xplat ui deploy  # Deploys to Cloudflare Workers
```

---

## xplat CLI Commands

### `xplat ui init`

Scaffolds a new Datastar TypeScript project:

```bash
xplat ui init my-dashboard

# Creates:
# my-dashboard/
# ├── src/
# │   ├── index.ts           # Hono app entry
# │   └── routes/
# │       └── index.ts       # Home route
# ├── package.json           # Dependencies
# ├── tsconfig.json          # TypeScript config
# ├── wrangler.toml          # CF Worker config
# ├── Taskfile.yml           # Task integration
# └── README.md              # Quick start guide
```

Options:
```bash
xplat ui init my-app --template dashboard   # Pre-built dashboard template
xplat ui init my-app --template chat        # Real-time chat template
xplat ui init my-app --template crud        # CRUD operations template
```

### `xplat ui dev`

Runs local development server:

```bash
cd my-dashboard
xplat ui dev

# Or via task (after init)
task ui:dev
```

Features:
- Hot reload on file changes
- SSE streaming works locally
- Same code that runs on CF Workers

### `xplat ui build`

Builds for production:

```bash
xplat ui build

# Output: dist/
```

### `xplat ui deploy`

Deploys to Cloudflare Workers:

```bash
xplat ui deploy

# Requires: wrangler.toml with account_id
```

---

## Generated Taskfile Integration

`xplat ui init` creates a Taskfile.yml that integrates with xplat patterns:

```yaml
# my-dashboard/Taskfile.yml
version: '3'

vars:
  UI_PORT: '{{.UI_PORT | default "3000"}}'

tasks:
  deps:
    desc: Install dependencies
    cmds:
      - bun install
    sources:
      - package.json
    generates:
      - bun.lockb

  dev:run:
    desc: Run development server with hot reload
    deps: [deps]
    cmds:
      - bun --watch src/index.ts

  build:
    desc: Build for production
    deps: [deps]
    cmds:
      - bun build src/index.ts --outdir dist

  deploy:
    desc: Deploy to Cloudflare Workers
    deps: [build]
    cmds:
      - wrangler deploy

  health:
    desc: Check if dev server is running
    cmds:
      - curl -sf http://localhost:{{.UI_PORT}}/health

  clean:
    desc: Clean build artifacts
    cmds:
      - rm -rf dist node_modules
```

Users can add `ui:` to their root Taskfile:

```yaml
# User's root Taskfile.yml
includes:
  ui: ./my-dashboard/Taskfile.yml
```

Then:
```bash
task ui:dev:run    # Run UI dev server
task ui:deploy     # Deploy to CF Workers
```

---

## Templates

### Dashboard Template

Real-time dashboard with SSE updates:

```bash
xplat ui init my-dashboard --template dashboard
```

Features:
- Live metrics cards
- Task/job list with status
- Activity feed

### Chat Template

Real-time chat application:

```bash
xplat ui init my-chat --template chat
```

Features:
- Message list with SSE updates
- Input with optimistic sending (Datastar way - shows loading, confirms on backend response)
- Typing indicators

### CRUD Template

Standard create/read/update/delete:

```bash
xplat ui init my-app --template crud
```

Features:
- List view with pagination
- Create/edit forms
- Delete confirmation
- Real-time list updates

---

## Integration with Go Services

Users' Go services can push SSE events that the TypeScript UI consumes:

### Go Service (user's existing code)

```go
// Using Datastar Go SDK
import "github.com/starfederation/datastar-go"

func handleStream(w http.ResponseWriter, r *http.Request) {
    sse := datastar.NewSSE(w)

    for event := range eventChannel {
        sse.PatchElements("#status", fmt.Sprintf(`
            <div id="status" class="text-green-500">%s</div>
        `, event.Message))
    }
}
```

### TypeScript UI

```typescript
// Connects to Go service's SSE endpoint
app.get('/status', (c) => c.html(`
  <div id="status"
       data-on-load="@get('http://localhost:8080/stream')"
       class="p-4">
    Connecting...
  </div>
`));
```

**Same SSE protocol** - Go and TypeScript Datastar SDKs are compatible.

---

## Why This Enables AI-First Development

### 1. Datastar Skills (ADR-011)

Claude Code has the Datastar skill installed, so it generates correct patterns:

```
User: "Add a form to create new tasks"

Claude: [Uses Datastar skill knowledge]
- Uses data-signals for form state
- Uses @post for submission
- Uses data-indicator for loading state
- Follows "backend is source of truth" principle
```

### 2. TypeScript = Better AI Output

AI models have more TypeScript training data than any other language. Generated code is:
- More likely to be correct
- Better typed
- Follows modern patterns

### 3. Instant Feedback Loop

```
Describe → Generate → Run → See Result → Iterate

# This loop takes seconds, not minutes
bun --watch   # Instant reload
```

### 4. Runtime Code Generation (Future)

```typescript
// Future: AI generates component at runtime
const userRequest = "Show me a chart of task completion over time";
const component = await ai.generateComponent(userRequest);
stream.patchElements(component);
```

---

## Implementation Plan

### Phase 1: Scaffolding

1. [ ] Create `xplat ui init` command
2. [ ] Create base template with Hono + Datastar SDK
3. [ ] Add Taskfile.yml generation
4. [ ] Test with Bun

### Phase 2: Templates

1. [ ] Dashboard template
2. [ ] Chat template
3. [ ] CRUD template
4. [ ] Template selection in `xplat ui init --template`

### Phase 3: Deployment

1. [ ] `xplat ui deploy` command
2. [ ] wrangler.toml generation
3. [ ] CF Worker environment variable handling

### Phase 4: AI Integration

1. [ ] Component generation helpers
2. [ ] Type-safe prompt templates
3. [ ] Example AI workflows in docs

---

## Technology Choices

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Runtime | Bun | Fast, TypeScript-native, built-in bundler |
| Router | Hono | Runs on Bun and CF Workers |
| Datastar SDK | @starfederation/datastar-sdk/web | Web Standards API |
| Edge runtime | Cloudflare Workers | Global, low-latency |
| Styling | Tailwind CSS (optional) | Utility-first, tree-shaken |

---

## Example: User Workflow

### 1. Initialize

```bash
cd my-project
xplat ui init dashboard --template dashboard
```

### 2. Describe to AI

```
User: "I want the dashboard to show:
       - A header with the project name
       - Real-time task count
       - Recent activity feed
       - Connect to my Go API at localhost:8080"
```

### 3. AI Generates Code

Claude generates `src/routes/dashboard.ts` using Datastar patterns.

### 4. Run Locally

```bash
task ui:dev:run

# Or with other services
task start  # process-compose includes UI
```

### 5. Deploy

```bash
xplat ui deploy
# → https://my-dashboard.workers.dev
```

---

## References

- Datastar: https://data-star.dev
- TypeScript SDK: https://github.com/starfederation/datastar-typescript
- Go SDK: https://github.com/starfederation/datastar-go
- Working example: https://github.com/m0hill/datastar-demo
- Hono: https://hono.dev
- Cloudflare Workers: https://developers.cloudflare.com/workers/
- Related: ADR-011 (Datastar skills for AI assistance)
