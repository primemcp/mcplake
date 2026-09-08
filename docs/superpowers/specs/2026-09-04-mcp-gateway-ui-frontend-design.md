# MCP Gateway UI frontend — design

- Status: Approved
- Date: 2026-09-04
- Source design: Claude Design project `mcplake`, file `MCP Gateway UI (standalone).html`
  (https://claude.ai/design/p/f6ce01ad-2551-493d-beed-4bb749ab5129?file=MCP+Gateway+UI+%28standalone%29.html)
- Design brief: `uploads/mcp-gateway-ui-design-brief.md` in that same project
- Base branch: `develop` (NOT `main` — see Context)

## Context

`main` in this repo is a stale Phase-1 scaffold (every backend function a `TODO`/
`not implemented` stub). The real Phase 1 backend is on `develop`, 49 commits ahead,
fully merged as of PR #51 ("Wire full application"): a `fasthttp` data-plane gateway,
a `Gin` control-plane admin API, GORM/SQLite persistence, and a unified JSONPath+regexp
claim-rule policy engine — see `docs/architecture/overview.md`, `components.md`, `data.md`
and ADR-0001 through ADR-0007 on `develop`. This spec targets `develop`.

The admin API is real and already exercised by tests:

```
POST/GET             /admin/mcps                        register/list an MCPRegistration
DELETE                /admin/mcps/:name                  unregister (no update — re-POST to change)
POST/GET             /admin/access-policies
GET/PUT/DELETE        /admin/access-policies/:name       { name, match: [{path,pattern}], grants: [{mcp, tools}] }
POST/GET             /admin/filter-policies
GET/PUT/DELETE        /admin/filter-policies/:name       { name, match, mcp, tool, drop_fields }
GET                   /admin/healthz
GET                   /admin/swagger/*any                 swaggo-generated OpenAPI UI
```

`config.ServerConfig` on `develop` has exactly two listen addresses —
`DataPlaneAddr` (fasthttp) and `ControlPlaneAddr` (Gin) — no third "UI port" exists.
There is no persisted `User` entity and no `Tags` field on `MCPRegistration`. This
spec's architecture and data-model sections were revised after discovering this (an
earlier draft assumed the stale `main` scaffold and invented a speculative API that
would have duplicated the real one).

The original design brief scoped three screens: a live pipeline graph, MCP instance
configuration, and claims/routing rule configuration. The artboard actually delivered
and named for this task — `MCP Gateway UI (standalone).html` — implements two screens
that evolved from that brief:

1. **MCP connections** — list/search of connected MCP endpoints; add/edit an endpoint
   (name, URL, transport, tags); per-endpoint response filters built from that
   endpoint's discovered response schema.
2. **Users & access** — list of gateway users; a detail panel with three tabs:
   - **Token match** — the JSONPath + regex that identifies this user from a JWT claim.
   - **Access** — which MCP endpoints (and which of their filters) this user is granted.
   - **Request path** — a per-user simulation of the auth → routing → filter pipeline,
     including the reject/401 state.

This spec covers implementing exactly those two screens as delivered, against the real
`develop` backend. The global live-graph screen and a standalone routing-rule-conflict
UI from the original brief are not part of this artboard and are out of scope here.

## Goals

- Implement the two screens pixel-faithfully against the Claude Design source, as a
  real, buildable frontend, wired to the real `develop` admin API — not a static trace
  of the mockup and not a speculative API of its own.
- Establish a component architecture that will still make sense once more admin
  screens are added later (the mockup reuses inline-edit forms, tag pickers, toggles,
  and dashed "add" buttons across both screens — these become shared primitives, not
  copy-pasted markup).
- Fit the project's existing "single Go binary, zero external dependencies at runtime"
  ethos: the built frontend ships embedded inside the gateway binary, served from the
  same Gin process (and same origin) as `/admin/*` — no new listener, no CORS.

## Non-goals

- Adding backend functionality that doesn't already exist: a persisted `User` table,
  an `MCPRegistration.Tags` field, or a `request-path-preview` simulation endpoint.
  Where the mockup implies one of these, the frontend composes existing primitives
  (`AccessPolicy` + `FilterPolicy`, in the User case — see Data model reconciliation)
  or drops the feature (endpoint tags — see same section).
- The live global pipeline-graph screen from the original design brief (not part of
  this artboard).
- Mobile/responsive layout — internal desktop operational tool, per the design brief.
- Any authentication/login UI for the admin UI itself (not part of this artboard). The
  admin API today has no auth of its own; that's a `develop`-side gap this spec doesn't
  touch.

## Data model reconciliation

Two mockup concepts have no backend counterpart. Resolved as follows (decided with the
project owner):

**"User" → paired `AccessPolicy` + `FilterPolicy` records, composed client-side.**
The mockup's "Users & access" screen keeps its current UX and terminology. Creating or
editing a user issues 2+ real API calls sharing one `Name` and one `Match`
(`ClaimMatcher`):
- One `AccessPolicy` (`POST/PUT /admin/access-policies[/:name]`) holding the `Grants`
  chosen on the Access tab.
- One `FilterPolicy` per response filter selected for this user
  (`POST/PUT /admin/filter-policies[/:name]`), each scoped to the `(mcp, tool)` the
  filter belongs to, sharing the same `Name`/`Match`.

A "user" is therefore a client-side view over policies that share a name — not a
persisted concept. `src/api/users.ts` documents this composition explicitly (see
Frontend structure) so it isn't mistaken for a real backend entity later. Listing users
means listing `AccessPolicy` records and joining in `FilterPolicy` records with a
matching name.

**Endpoint tags → dropped from the UI.** `MCPRegistration` has no `Tags` field. The
MCP connections screen is implemented without the tag picker/chips from the mockup
(search still works, against name/URL). Not proposed as a backend addition here —
flagged in this doc for whoever next touches `persistence`/`controlplane`/`mcps.go`,
but out of scope for this task.

## Architecture

### Where the UI is served from

No new Go module, no new port. The built frontend is embedded into and served by the
**existing** `gateway/internal/controlplane` package (the Gin control-plane server
already wired in `cmd/gateway/app/app.go`):

- `controlplane.Server` gains an `Engine() *gin.Engine` accessor (small, non-breaking
  addition next to the existing `Admin()` accessor).
- A new `gateway/internal/controlplane/webui.go` calls
  `//go:embed webui/dist` and exposes `RegisterUIRoutes(engine *gin.Engine, assets fs.FS)`,
  which serves `webui/dist`'s static files and falls back to `index.html` for any
  unmatched non-`/admin` path (`engine.NoRoute(...)`), so the SPA and the JSON API
  share one listener, one origin, one `ControlPlaneAddr`.
- `app.go`'s `New()` gains one line: `controlplane.RegisterUIRoutes(controlPlane.Engine(), webui.Assets)`,
  next to the existing `RegisterMCPRoutes`/`RegisterAccessPolicyRoutes`/
  `RegisterFilterPolicyRoutes` calls.

`go:embed` cannot reach outside its own package directory, which is why the frontend
project physically lives under `controlplane/`, not at the repo root:

```
gateway/
  internal/controlplane/
    server.go            # existing; gains Engine() accessor
    webui.go              # new; //go:embed webui/dist, RegisterUIRoutes
    webui/                 # NOT a Go module — a Bun/Vite/React project
      package.json
      vite.config.ts
      tailwind.config.ts
      tsconfig.json
      index.html
      src/
        main.tsx
        App.tsx                        # AppShell composition root
        theme.css                      # design tokens as CSS custom properties
        api/
          client.ts                    # fetch wrapper, base path "/admin"
          types.ts                     # hand-written types mirroring the Go DTOs
                                        # in mcps.go / access_policies.go / filter_policies.go
          endpoints.ts                 # useEndpoints(), useEndpoint(name)
          policies.ts                  # useAccessPolicies(), useFilterPolicies()
          users.ts                     # useUsers()/useUser(name): composes
                                        # AccessPolicy+FilterPolicy into the UI's
                                        # "User" shape; documents the composition
        components/
          primitives/
            Button.tsx
            Input.tsx
            SearchInput.tsx
            Toggle.tsx
            Card.tsx
            Tabs.tsx
            EmptyState.tsx
            StatusDot.tsx
          instances/
            EndpointList.tsx           # left column: search + list
            EndpointDetail.tsx         # header, edit form, enable/disable toggle
            ResponseFilterGroup.tsx    # filter list + inline add/edit + schema field picker
                                        # (schema fields come from Endpoint.tools[tool]
                                        # .output_schema, already returned by GET /admin/mcps
                                        # — no separate schema endpoint needed)
          users/
            UserList.tsx               # left column: composed users
            UserDetail.tsx             # tab container
            TokenMatchTab.tsx
            AccessTab.tsx
            RequestPathTab.tsx         # client-side pipeline trace (see below)
        layout/
          Sidebar.tsx                  # nav, schema-cache summary, health footer
          AppShell.tsx
        state/
          useNav.ts                   # which screen is active (goInstances / goUsers)
      dist/                            # build output, git-ignored, embedded by webui.go
```

`webui` deliberately has no `go.mod` — it's an npm/Bun project; Go code only embeds its
build output, never imports its source.

**Request path tab**: since there is no `request-path-preview` backend endpoint (Non-
goals), this tab computes its trace **client-side**, using data already fetched for the
user: the matching `AccessPolicy`'s `Grants` and `FilterPolicy`(s) plus the same
`ClaimRule` matching semantics described in `docs/architecture/data.md` (JSONPath
extraction + regexp, list-valued paths matched if any element matches), reimplemented
in TypeScript against a claim JSON the operator pastes/edits in the tab. This is a
simulation against the currently-saved policies, not a live trace of a real request —
labeled as such in the UI so it isn't confused with an audit log (Phase 3, explicitly
out of scope project-wide).

### Why this shape

- **No new Go module or port**: the control-plane server already exists, already has a
  route-group extension point (`Admin()`), and the project's whole ethos is one
  listener per concern, not one per screen. Mounting the SPA here is strictly less
  than the alternative (new module, new config field, new port, CORS).
- **No client-side router**: the mockup's own navigation (`goInstances`/`goUsers`) is
  in-memory state, not URL-addressed. A router for two screens with no deep links
  would be premature abstraction (`.claude/skills/development` conventions).
- **No global state library**: nothing here needs cross-screen shared mutable state
  beyond "which screen is active" (`AppShell`). Each hook owns its own fetch/loading/
  error state. Revisit if/when a third screen needs to share fetched data.
- **Hand-written types over generated client**: the admin API is 3 resources, ~12
  routes, already fully specified in `data.md` and the Go DTOs. A swagger-to-TS
  codegen step is available later if the surface grows, but isn't justified yet
  (`docs/architecture` and `.claude/skills/development` both favor standard-library/
  minimal-dependency solutions over adding tooling for a small, stable surface).
- **Primitives extracted, not duplicated**: the mockup repeats specific patterns
  byte-for-byte across both screens (tag chip, toggle switch, dashed "+ Add ..."
  button, inline `data-inline-form` panel). Each becomes exactly one component.

## Error handling

- Every hook (`useEndpoints`, `useAccessPolicies`, `useUser`, ...) returns
  `{ data, loading, error, retry }` and never throws into the component tree.
- Each screen section renders its own inline failure state ("Couldn't reach the
  gateway admin API" + retry) scoped to what failed.
- One top-level `ErrorBoundary` wraps `AppShell` to catch genuine render bugs (not
  expected API failures).

## Testing

- **Frontend**: Vitest + React Testing Library.
  - Primitives: `Toggle` flips and reports state, `SearchInput` filters a list,
    `Tabs` switches panels.
  - Screens: `EndpointList` search narrows the list; `EndpointDetail` edit form
    validates required fields before enabling Save; `RequestPathTab` renders the
    reject state distinctly (per the design brief: a red/alert accent in the light
    theme, not decorative).
  - `users.ts`: the `AccessPolicy`+`FilterPolicy` → "User" composition is the one
    piece of real client-side logic here and gets direct unit tests — matching by
    name, handling a user with an access policy but no filter policies yet, etc.
  - API hooks: mock `fetch`, assert loading → error → retry and loading → data.
- **Go (`gateway/internal/controlplane`)**: a `webui_test.go` verifying `GET /` serves
  the embedded `index.html`, an unmatched path falls back to it (SPA routing), and
  `/admin/*` is untouched by the new catch-all — same lightweight style as the
  existing `mcps_test.go`/`access_policies_test.go`.
- No end-to-end tests in this pass.

## Workspace / build integration

- No `go.work` changes — `webui` isn't a Go module.
- New Makefile targets: `ui-dev` (`bun --cwd gateway/internal/controlplane/webui run dev`,
  Vite dev server proxying `/admin` to a locally-running gateway) and `ui-build`
  (`bun --cwd gateway/internal/controlplane/webui run build`, producing `webui/dist`
  for `webui.go` to embed). `make build` depends on `ui-build`.
- `.devcontainer/Containerfile` updated to install `bun` (done as prep work ahead of
  this spec).
