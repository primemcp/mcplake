# Components

This supersedes the component list in [`/docs/OVERVIEW.md`](../OVERVIEW.md#core-components)
for the pieces this milestone touches. Modules not mentioned here (e.g. `config`) keep
their existing responsibility, extended only to carry the new rule/policy shapes
described in [`data.md`](data.md).

## Gateway Server — Data Plane (`gateway`)

- Built on `fasthttp` ([ADR-0001](decisions/0001-use-fasthttp-for-gateway-server.md)).
- Owns exactly one route family: the tool-call endpoint used by agents. Admin
  operations live on the separate control-plane surface below.
- Per request, orchestrates (in order): Auth Validator → Policy Engine (access) →
  MCP Registry/Router → MCP Client → Policy Engine (filter) → Response Filter.
- Holds no policy or routing logic itself — it is wiring, not a decision-maker. This
  keeps the decision logic (`auth`, `router`, `filter`) testable without an HTTP
  server in the loop.
- Reads the Policy Engine and MCP Registry from their in-memory caches only — never
  queries the persistence layer directly, to keep the per-request cost independent of
  the database (see [Persistence Layer](#persistence-layer-persistence) below).

## Control-Plane API (`Gin`)

- Built on Gin ([ADR-0005](decisions/0005-use-gin-for-control-plane-api.md)), run as
  a second HTTP surface with its own listener, separate from the data plane.
- Owns CRUD for `MCPRegistration`, `AccessPolicy`, and `FilterPolicy`: `/admin/mcps`,
  `/admin/access-policies`, `/admin/filter-policies`.
- The Gin handlers are thin adapters over `adminservice` — a transport-agnostic
  application layer holding the write-through ordering, validation, and typed
  errors ([ADR-0011](decisions/0011-mcp-control-server.md)).
- Every write goes through the Persistence Layer and then triggers an in-memory
  cache refresh in the MCP Registry / Policy Engine, so the data plane observes
  changes without querying the database itself.
- Authenticated when `admin_auth` is configured: a Gin middleware verifies a
  Bearer JWT and claim-gates it, covering every `/admin/*` route except
  `GET /admin/healthz` ([ADR-0010](decisions/0010-control-plane-admin-authentication.md)).
- Intended consumers: human operators (via `curl`/scripts), the Admin Web UI's
  backend calls (below), and — when `admin_mcp.enabled` — MCP clients via the
  MCP Control Server (below).
- Not on the tool-call hot path; ergonomics (request binding/validation, clear
  routing) are prioritized over raw throughput here, unlike the data plane.
- Documented with `swag`-generated OpenAPI, served via `gin-swagger` at
  `/admin/swagger/index.html` on the same control-plane port, behind the same trust
  boundary as the rest of `/admin/*`
  ([ADR-0007](decisions/0007-swaggo-for-control-plane-api-docs.md)).

## MCP Control Server (`gateway/internal/controlplane/adminmcp`)

- An in-process MCP server that exposes every Control-Plane API operation as an
  MCP tool (`list_mcps`, `register_mcp`, `set_mcp_enabled`, `unregister_mcp`, the
  access/filter-policy CRUD, `gateway_health`), so an MCP client can administer a
  running gateway ([ADR-0011](decisions/0011-mcp-control-server.md)).
- Built on the `modelcontextprotocol/go-sdk` server API; a thin adapter over the
  same `adminservice` layer the Gin handlers use, so the two surfaces cannot
  drift. A service error becomes an MCP tool error, not a protocol error.
- Mounted as a streamable-HTTP handler on the control-plane listener under
  `/admin/` (default `/admin/mcp`), so it inherits the `admin_auth` gate. Off by
  default; enabled with `admin_mcp.enabled`.
- Full tool catalog and usage: [`api/admin-mcp.md`](../api/admin-mcp.md).

## Persistence Layer (`persistence`)

- GORM-based ([ADR-0006](decisions/0006-gorm-sqlite-postgres-persistence.md)),
  SQLite by default (pure-Go driver, no CGO), PostgreSQL for distributed
  deployments — selected by configuration, same model definitions either way.
- Sole owner of durable state for `MCPRegistration`, `AccessPolicy`, and
  `FilterPolicy`. Nested rule/grant data is stored as JSON text columns so the same
  schema works unchanged across both dialects.
- Startup reconciles `config.yaml`'s static `mcps:`/`access_policies:`/
  `filter_policies:` entries into this store (upsert), so config becomes a seed
  mechanism rather than a second source of truth.
- Never touched by the data plane directly — only by the Control-Plane API (writes)
  and by the MCP Registry / Policy Engine's cache-load-on-startup and
  cache-refresh-on-write paths (reads).

## Auth Validator (`auth`)

- Verifies JWT signature against the OIDC provider's cached JWKS.
- Validates standard claims (`exp`, `iat`, `iss`, `aud`).
- Decodes the full claim set into a generic JSON document (not a flattened
  `map[string]string`) so the Policy Engine can run JSONPath expressions against
  nested claims (e.g. `$.groups[*]`, `$.org.tier`).
- Returns 401 on any validation failure. Does not make authorization decisions — it
  only proves who the caller is.

## Policy Engine (`router`, new: claim rule matching)

- Evaluates `ClaimRule` conditions (JSONPath extraction + regexp match) against a
  claim set. See [ADR-0002](decisions/0002-jsonpath-regexp-claim-rule-engine.md) for the
  matching semantics, including how list-valued JSONPath results are handled.
- Two policy kinds share this same matcher, per
  [ADR-0004](decisions/0004-unified-policy-engine-for-access-and-filtering.md):
  - **Access policies** — match → grant a set of `(mcp, tool)` pairs.
  - **Filter policies** — match → contribute a set of response field paths to drop
    for a given `(mcp, tool)`.
- Given a claim set, `Router.Authorize(claims, mcp, tool) (bool, error)` answers whether
  the call is permitted (union of all matching access policies).
- Given a claim set and a `(mcp, tool)`, `Router.FieldsToRemove(claims, mcp, tool) ([]string, error)`
  answers which fields the response filter must strip (union of all matching filter
  policies).

## MCP Registry (`cache`, extended)

- In-memory, read-optimized view over the `MCPRegistration` rows in the
  [Persistence Layer](#persistence-layer-persistence): connection config, live
  `mcp.Client`, and the tool/schema list fetched at registration time. This cache,
  not the database, is what the data plane consults per request.
- Registration flow detailed in
  [ADR-0003](decisions/0003-dynamic-mcp-registration-and-schema-discovery.md):
  connect → `tools/list` → persist `{name, inputSchema, outputSchema}` per tool →
  mark MCP `active` → refresh the in-memory cache.
- Supports registration both from static config at startup and from the
  Control-Plane API at runtime, through the same internal `Register()` path — there
  is no separate "static" vs. "dynamic" code path; both end up as a row in the same
  store.
- Exposes read APIs for the Policy Engine (does this MCP/tool exist?) and for the
  future Admin/Graph UI (what's registered, what tools does it expose?).

## MCP Client (`mcp`)

- One client instance per registered MCP; unchanged in responsibility from
  `/docs/OVERVIEW.md`, but its `ListTools` result is now the input to the Registry's
  schema cache rather than being discarded after startup.
- Each tool call runs with a per-call `context.Context` carrying a timeout/deadline
  independent of the inbound fasthttp request lifecycle (fasthttp's `RequestCtx` is
  pooled and reused after the handler returns, so it must not be retained — see
  [ADR-0001](decisions/0001-use-fasthttp-for-gateway-server.md#consequences)).

## Admin Web UI (`gateway/internal/controlplane/webui`)

- React 19 + TypeScript SPA, built with Vite and embedded into the control-plane
  binary via `//go:embed` — served from the same listener and port as the
  Control-Plane API, no separate origin/CORS setup. See
  [features/admin-webui.md](../features/admin-webui.md) for what it does and how
  to run it.
- Talks to the real `/admin/*` API documented in
  [api/admin.md](../api/admin.md) — no separate backend or mock layer; every
  screen reflects live Registry/Policy Engine state.
- One place where the UI's data model doesn't map 1:1 onto the backend's: a
  response filter can span several tools from the operator's point of view, even
  though `FilterPolicy` (above) is always single-tool — reconciled entirely in
  the frontend, see [ADR-0008](decisions/0008-frontend-only-multi-tool-filter-grouping.md).

## Response Filter (`filter`)

- Takes the raw tool response plus the field list from
  `Router.FieldsToRemove(...)` and removes those fields from the JSON body.
- Uses the cached output schema (from the MCP Registry) only to validate that the
  drop paths are structurally sensible (e.g. warn/skip on a path that doesn't exist
  in the schema) — the schema is a safety check, not the source of what to drop.
  What to drop always comes from claims via the Policy Engine, not from the schema.
