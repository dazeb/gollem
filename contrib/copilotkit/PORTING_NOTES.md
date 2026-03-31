# CopilotKit Porting Notes for gollem AGUI UI

Upstream snapshot inventoried in `repomix-output.xml`:

- Repo: `https://github.com/CopilotKit/CopilotKit`
- Commit: `ecf67473ad08c47af89a5289839b518c422dc15f`
- License: MIT
- Upstream NOTICE file: none found in this snapshot

Local gollem files reviewed for fit/gaps:

- `cmd/gollem/serve.go`
- `ext/agui/DESIGN.md`
- `pkg/ui/server.go`
- `pkg/ui/handlers.go`
- `pkg/ui/state.go`
- `pkg/ui/templates/layout.html`
- `pkg/ui/templates/index.html`
- `pkg/ui/templates/run.html`
- `pkg/ui/templates/sidebar.html`
- `pkg/ui/static/renderer.js`
- `pkg/ui/static/style.css`

Note: the task referenced `ext/agui/MISSION-AGUI-UI.md`, but that file is not present in this worktree, so the notes below are based on the files that do exist.

## Minimal conclusion

CopilotKit is useful here mainly as:

1. a **behavior reference** for chat/approval/interrupt UX,
2. a **layout/style reference** for chat + sidebar + canvas surfaces,
3. a **semantic reference** for A2UI/AGUI event handling.

It is **not** a drop-in source transplant for gollem. The React provider/hook tree, Next.js app structure, NX workspace plumbing, Tailwind/shadcn/Radix composition, and JSX rendering model must be re-expressed in Go templates plus vanilla JS.

## Exact upstream areas worth copying or adapting

### 1. Chat shell composition and CSS behavior

**Upstream files**

- `packages/react-ui/src/components/chat/Chat.tsx`
- `packages/react-ui/src/components/chat/Messages.tsx`
- `packages/react-ui/src/components/chat/Input.tsx`
- `packages/react-ui/src/components/chat/Sidebar.tsx`
- `packages/react-ui/src/css/messages.css`
- `packages/react-ui/src/css/sidebar.css`
- `packages/react-ui/src/styles.css`

**Exact pieces to copy/adapt**

- `Messages.tsx` `useScrollToBottom(...)` behavior:
  - keep auto-scroll when new assistant content arrives,
  - stop forcing scroll when the user has manually scrolled up,
  - resume sticky-bottom behavior on a fresh user turn.
- `Input.tsx` send/stop/input semantics:
  - Enter to submit,
  - Shift+Enter for newline,
  - IME composition safety,
  - send vs stop button state,
  - disabled state while run/tool interruption is active.
- `Sidebar.tsx` shell idea:
  - collapsible side control plane,
  - open/closed state styling,
  - pairing a control sidebar with a main run surface.
- `messages.css` / `sidebar.css` structure and spacing tokens:
  - message list spacing,
  - footer/input layout,
  - sidebar width/expansion rhythm,
  - assistant vs user message visual separation.

**Best gollem landing zones**

- `pkg/ui/templates/run.html`
- `pkg/ui/templates/sidebar.html`
- `pkg/ui/static/style.css`
- `pkg/ui/static/renderer.js`

**Porting guidance**

- Copy **interaction rules and CSS proportions**, not JSX.
- Rebuild markup in Go templates.
- Rebuild event/state handling in vanilla JS against gollem SSE and `data-*` hydration.

### 2. Interrupt / approval / human-in-the-loop semantics

**Upstream files**

- `packages/react-core/src/v2/hooks/use-interrupt.tsx`
- `packages/react-core/src/v2/hooks/use-human-in-the-loop.tsx`
- `packages/react-core/src/v2/hooks/use-agent.tsx`
- `packages/react-core/src/v2/providers/CopilotKitProvider.tsx`

**Exact pieces to adapt**

- `use-interrupt.tsx`
  - custom-event driven interrupt capture,
  - hold interrupt UI until run-finalization point,
  - explicit `resolve(...)` path back into the runtime,
  - dual rendering modes: in-chat vs external panel.
- `use-human-in-the-loop.tsx`
  - tool renderer states: in-progress / executing / complete,
  - explicit `respond(...)` continuation contract,
  - cleanup when renderer unmounts.
- `use-agent.tsx`
  - stable subscription model around run status/messages/state updates.
- `CopilotKitProvider.tsx`
  - only as a semantic reference for:
    - runtime connection status,
    - tool-call renderer registration,
    - A2UI enablement,
    - error/diagnostic surfacing.

**Best gollem landing zones**

- `ext/agui/adapter.go`
- `ext/agui/transport/sse.go`
- `pkg/ui/handlers.go`
- `pkg/ui/state.go`
- `pkg/ui/static/renderer.js`

**Porting guidance**

- Map React interrupt/human-loop concepts onto existing gollem runtime surfaces:
  - `RunRecord.Session()` / AGUI session state,
  - `RunRecord.ApprovalBridge()`,
  - `transport.NewActionHandler(...)`,
  - replay-safe SSE in `ext/agui/transport/sse.go`.
- The portable asset is the **state machine**, not the hook API.

### 3. A2UI surface rendering concepts

**Upstream files**

- `packages/react-core/src/v2/a2ui/A2UIMessageRenderer.tsx`
- `packages/a2ui-renderer/src/react-renderer/core/A2UIRenderer.tsx`
- `packages/a2ui-renderer/src/react-renderer/styles/README.md`
- `packages/a2ui-renderer/src/react-renderer/**`
- `packages/a2ui-renderer/src/theme/**`

**Exact pieces to adapt**

- Group incoming operations by `surfaceId` before rendering.
- Treat actions on the rendered surface as explicit runtime messages back into the agent loop.
- Preserve the idea of a renderer-local style boundary/reset so host CSS cannot accidentally break A2UI semantics.
- Reuse theme concepts:
  - palette variables,
  - surface-level CSS custom properties,
  - component-vs-utility style layering.

**Best gollem landing zones**

- `pkg/ui/static/renderer.js`
- `pkg/ui/static/style.css`
- future A2UI-specific Go template fragments if gollem grows beyond the current run scene

**Porting guidance**

- Port the **surface/operation model** and **style-isolation strategy**.
- Do **not** port the React component registry or JSX component tree directly.
- If gollem later supports generic A2UI rendering, build a vanilla DOM/canvas renderer around Gollem's existing AGUI/SSE model rather than trying to emulate React hooks.

### 4. Example references worth borrowing from

#### Todo showcase

**Upstream files**

- `examples/showcases/todo/src/app/page.tsx`
- `examples/showcases/todo/src/components/TodoItem.tsx`

**Useful patterns**

- expose app state to the agent (`useCopilotReadable`) -> maps conceptually to gollem run snapshots / scene state,
- expose typed actions/tools to mutate UI state (`useCopilotAction`) -> maps to gollem action routes and approval/deferred flows,
- compact task-oriented chat UI.

**Best gollem landing zones**

- `pkg/ui/templates/index.html`
- `pkg/ui/templates/run.html`
- `pkg/ui/handlers.go`
- `pkg/ui/state.go`

#### Multi-agent canvas showcase

**Upstream files**

- `examples/showcases/multi-agent-canvas/frontend/src/components/chat-window.tsx`
- `examples/showcases/multi-agent-canvas/frontend/src/components/canvas.tsx`
- `examples/showcases/multi-agent-canvas/frontend/src/components/app-sidebar.tsx`
- `examples/showcases/multi-agent-canvas/frontend/src/app/globals.css`

**Useful patterns**

- split layout: chat/control panel + large visual workspace,
- currently-running-agent status badge,
- multi-panel composition around agent execution,
- richer scene-oriented work area instead of plain transcript-only UI.

**Best gollem landing zones**

- `pkg/ui/templates/run.html`
- `pkg/ui/templates/sidebar.html`
- `pkg/ui/static/style.css`
- `pkg/ui/static/renderer.js`

## Exact upstream areas that are non-portable and must be reimplemented

These should **not** be copied as-is into gollem.

### React / framework-specific

- `packages/react-ui/src/components/chat/*.tsx` JSX component trees
- `packages/react-core/src/v2/providers/*.tsx` React context/provider wiring
- `packages/react-core/src/v2/hooks/*.tsx` hook APIs as public shape
- Next.js app/router/layout files under `examples/**/src/app/**`
- all `"use client"` components

**Why non-portable:** gollem serves server-rendered Go templates and vanilla JS, not React components.

### NX / monorepo / package plumbing

- `package.json`
- `nx.json`
- workspace scripts/build graph
- `postcss`, `tsdown`, `pnpm`, Storybook, package export wiring

**Why non-portable:** gollem is a Go repo and should not inherit JS monorepo mechanics for this UI.

### Tailwind / shadcn / Radix / icon dependencies

- Tailwind configs in examples
- shadcn-style utility class composition
- Radix dialog/sidebar component composition
- Lucide icon component imports

**Why non-portable:** these can inspire class naming and spacing, but should be rewritten into native HTML/CSS/JS in `pkg/ui/templates/*` and `pkg/ui/static/style.css`.

### React-specific A2UI renderer internals

- component registry wiring in `packages/a2ui-renderer/src/react-renderer/registry/**`
- React provider/store hooks in `packages/a2ui-renderer/src/react-renderer/core/**`
- Suspense/lazy-component patterns

**Why non-portable:** gollem needs the operation semantics, not the React rendering substrate.

## Portable vs. re-express matrix

| Area | Portable as-is? | What to do in gollem |
| --- | --- | --- |
| MIT-licensed CSS snippets | Sometimes | Copy selectively, preserve attribution in `contrib/copilotkit/NOTICE`, and normalize class names to gollem templates/CSS |
| Scroll/input interaction rules | Yes, as logic | Re-express in `pkg/ui/static/renderer.js` |
| Layout proportions and visual hierarchy | Yes, as design | Rebuild in `layout.html`, `run.html`, `sidebar.html`, `style.css` |
| Approval/interrupt state machine | Yes, as semantics | Map onto `pkg/ui/state.go`, action handlers, and SSE events |
| A2UI operation grouping / surface idea | Yes, as semantics | Rebuild in vanilla JS renderer |
| React providers/hooks | No | Reimplement against gollem AGUI session/store/runtime |
| Next.js routes/layouts | No | Reimplement in Go handlers/templates |
| NX/Tailwind/shadcn scaffolding | No | Ignore and restyle natively |

## Recommended minimal porting plan

1. **Do copy/adapt first**
   - `Messages.tsx` scroll behavior
   - `Input.tsx` send/stop/Enter semantics
   - `Sidebar.tsx` open/close shell pattern
   - `A2UIMessageRenderer.tsx` surface grouping + action bridge semantics
   - A2UI style-boundary ideas from the renderer style README

2. **Do not copy yet**
   - provider trees,
   - example app routers,
   - Tailwind/shadcn utilities,
   - package/build files.

3. **Re-express in gollem terms**
   - session/replay/approval flows remain driven by:
     - `ext/agui/adapter.go`
     - `ext/agui/transport/sse.go`
     - `pkg/ui/state.go`
     - `pkg/ui/handlers.go`
     - `pkg/ui/static/renderer.js`

## Attribution and license handling requirements

CopilotKit is MIT licensed. If any CopilotKit source, CSS, or other substantial portions are copied into gollem:

1. keep the upstream copyright and MIT permission notice,
2. retain this attribution record in `contrib/copilotkit/NOTICE`,
3. record the upstream repo URL + commit used for the copy,
4. note in code review/PR text which files were copied or materially adapted,
5. prefer file-header comments for near-verbatim copied code blocks,
6. if only ideas/behavior are reimplemented from scratch, attribution is still good practice but the MIT notice requirement is triggered by copying substantial portions of the original material.

## Practical attribution checklist for future code copies

When copying from any file listed in `repomix-output.xml`:

- add/update `contrib/copilotkit/NOTICE`,
- include the original upstream path and commit in a nearby comment or commit message,
- keep copied blocks distinguishable from gollem-native code,
- avoid pulling in React-only scaffolding when only behavior or CSS is needed.
