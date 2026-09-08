# Admin Web UI

A React 19 + TypeScript SPA for operating the gateway without hand-writing
`curl` calls against the [Control-Plane API](../api/admin.md): registering MCP
endpoints and managing their response filters. Embedded into the control-plane
binary and served from the same listener/port — no separate origin, no CORS
configuration, nothing extra to deploy.

Source: `gateway/internal/controlplane/webui/`.

## Running it

**Production (embedded):** built automatically as part of `make build` (or
standalone via `make ui-build`, which writes `webui/dist`, embedded by
`gateway/internal/controlplane/webui.go` via `//go:embed`). The compiled
gateway binary serves it directly — nothing to run separately.

**Local frontend development:** `make ui-dev` starts Vite's dev server, which
proxies `/admin` to a gateway control-plane already running locally (default
`http://localhost:8081` — see `vite.config.ts`), so the dev server talks to
the real admin API instead of a mock. Start the gateway first
(`make build && ./cmd/gateway/mcp-gateway --config <your config>`), then
`make ui-dev` in a second terminal.

**Tests:** `make ui-test` (`bun install --frozen-lockfile && bun run test`) —
Vitest + React Testing Library, no live gateway required (each test mocks
`fetch` directly, see `src/api/client.test.ts` for the pattern). Type-checking
runs as part of `make ui-build` (`tsc -b && vite build`).

## Screens

### MCP connections

Endpoint list (left column) + detail/edit panel + response filters, for the
selected endpoint.

- **Add/edit an endpoint** registers via `POST /admin/mcps` (there is no
  `PUT` — editing re-registers under the same name, which the backend
  upserts). Only `stdio` transport actually works against the real gateway
  today (`sse`/`http` are shown, explicitly disabled, not hidden — the
  backend doesn't implement them yet); the form collects a command +
  arguments accordingly, not a URL.
- **Response filters** are built from the endpoint's discovered tool schemas
  (`GET /admin/mcps` already returns each tool's `output_schema` — no
  separate schema-discovery call). The field picker merges every tool's
  fields into one searchable, toggleable list; a field can be toggled
  regardless of which tool it belongs to, and **one filter can span several
  tools** even though the backend's `FilterPolicy` is always single-tool —
  see [ADR-0008](../architecture/decisions/0008-frontend-only-multi-tool-filter-grouping.md)
  for how that's reconciled into real per-tool records without any backend
  change. Filter names may not contain `::` (reserved for that grouping
  convention — enforced in the UI, not just documented).
- **Schema graph**: any field with nested children shows a `[N]` badge (N =
  total descendant fields, recursively) that opens an interactive
  node/edge graph of that tool's full response shape (`@xyflow/react` +
  `@dagrejs/dagre` for layout). Clicking an edge toggles whether that field
  is dropped — the same underlying selection state as the flat list, just a
  more legible way to browse/edit a deeply nested or heavily branching
  schema than a flat list would allow.

### Users & access

Not yet implemented — tracked as [#80](https://github.com/atsokha/mcplake/issues/80)
(composing a "user" from an `AccessPolicy` + its same-named `FilterPolicy`
records) and [#81](https://github.com/atsokha/mcplake/issues/81) (a
client-side request-path simulation tab).

## Data model notes specific to the UI

- **Schema flattening and search** (`lib/schema.ts`) recurses through nested
  objects and arrays-of-objects using the same JSONPath `[*]` wildcard
  syntax `filter.Strip()` actually matches against (e.g.
  `$.entities[*].name`) — verified against a real 15-level, branching test
  fixture, not just shallow examples. Arrays of primitives/arrays are
  leaves (not expandable), described recursively (`array<array<string>>`).
- **The `[N]` badge** (`descendantCount`) counts every field nested under a
  row, recursively — not how many levels deep the nesting goes. A row with
  two direct leaf children reads `[2]`, the same as a row with one child
  that itself has one child.
- **Multi-tool filter grouping** (`lib/filterGroups.ts`): see
  [ADR-0008](../architecture/decisions/0008-frontend-only-multi-tool-filter-grouping.md).

## Security

A same-repo security audit (2026-09-05) of this feature found and fixed two
issues, both now covered by regression tests:

- A filter name containing `::` could silently merge a new filter into an
  unrelated existing filter's real records on save (see ADR-0008's Risks
  section) — fixed by rejecting `::` in the name input.
- The field picker's internal `(tool, path)` composite key was
  delimiter-joined with a plain space, which a tool name containing a space
  (nothing validates tool-name charset anywhere on the backend — see
  [ADR-0003](../architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery.md))
  would silently misparse, saving a filter against a tool that doesn't
  exist — fixed by JSON-encoding the pair instead of delimiter-joining it.

The admin API itself has no authentication (see
[api/admin.md](../api/admin.md)) — this UI inherits that trust boundary
unchanged. Bind the control-plane listener to a trusted network/interface
only; the webui does not add or need its own auth layer.
