# ADR-0011: In-Process MCP Control Server for Gateway Administration

- Status: Accepted
- Date: 2026-09-10

## Context

The gateway sits between AI agents and downstream MCP servers. Today the only
way to *operate* it — register/enable/disable/remove downstream MCPs, CRUD
access and filter policies — is the Gin control-plane REST API
([ADR-0005](0005-use-gin-for-control-plane-api.md)). An AI agent that manages the
gateway has to hand-roll HTTP calls against that API.

Exposing the same operations as **MCP tools** lets an MCP-speaking client (an
agent, an operator's MCP-enabled editor) administer the gateway through the same
protocol it already uses to reach tools. Requirements:

- **Behavioural parity.** A tool call and the equivalent REST call must do
  exactly the same thing — the same write-through ordering (live in-memory
  Registry / PolicyStore first, then the GORM store), the same validation, the
  same reload of the shared policy engine. Divergence between two
  implementations of "register an MCP" is a latent correctness bug.
- **Reuse the existing dependency.** `github.com/modelcontextprotocol/go-sdk` is
  already a workspace dependency — `mcp/` uses its *client* side to talk to
  downstream MCPs. It also provides a server (`mcp.NewServer`, `mcp.AddTool`,
  `mcp.NewStreamableHTTPHandler`).
- **Authentication.** Administering the gateway is at least as sensitive as the
  REST admin API, so the MCP control surface must sit behind the same gate as
  `/admin/*` ([ADR-0010](0010-control-plane-admin-authentication.md)).
- **Opt-in.** A gateway that doesn't want this surface should not expose it.
- **No new module, no new listener** if avoidable — the control plane is already
  a second listener; a third would be more to operate, document, and secure.

Non-goals: exposing the data-plane `POST /v1/call` as a tool (this is a *control*
surface); MCP prompts or resources; a standalone admin CLI.

## Decision

Add an in-process MCP server that exposes each control-plane admin operation as
one tool, built on the already-vendored go-sdk, and mounted on the existing
control-plane listener behind the ADR-0010 admin-auth middleware.

### Shared application layer

Extract a transport-agnostic package `adminservice` (under
`gateway/internal/controlplane`) from the current Gin handlers:

- `MCPService` — `List`, `Register`, `SetEnabled`, `Unregister`, keeping the
  "live Registry first, then repository" write-through that `mcps.go` performs
  today.
- `AccessPolicyService` / `FilterPolicyService` — `List`, `Get`, `Upsert`,
  `Delete`, each followed by `PolicyReloader.Refresh` so the data plane sees the
  change immediately.
- Typed errors (`ErrNotFound`, `ErrInvalidPolicy`, `ErrRegistrationFailed`,
  `ErrInvalidRequest`) checked with `errors.Is`.

The Gin handlers become thin adapters: bind the request DTO, call the service,
map a typed error to an HTTP status + `{"error","message"}` body. The MCP tools
are a second adapter over the same services. The existing
`gateway/internal/controlplane` handler tests are the HTTP-contract regression
guard and must keep passing unchanged.

### Tool surface (1:1 with the REST API)

| Tool | REST equivalent |
|------|-----------------|
| `list_mcps` | `GET /admin/mcps` |
| `register_mcp` | `POST /admin/mcps` |
| `set_mcp_enabled` | `PATCH /admin/mcps/:name` |
| `unregister_mcp` | `DELETE /admin/mcps/:name` |
| `list_access_policies` | `GET /admin/access-policies` |
| `get_access_policy` | `GET /admin/access-policies/:name` |
| `create_access_policy` | `POST /admin/access-policies` |
| `replace_access_policy` | `PUT /admin/access-policies/:name` |
| `delete_access_policy` | `DELETE /admin/access-policies/:name` |
| `list_filter_policies` | `GET /admin/filter-policies` |
| `get_filter_policy` | `GET /admin/filter-policies/:name` |
| `create_filter_policy` | `POST /admin/filter-policies` |
| `replace_filter_policy` | `PUT /admin/filter-policies/:name` |
| `delete_filter_policy` | `DELETE /admin/filter-policies/:name` |
| `gateway_health` | `GET /admin/healthz` (+ a small status summary) |

Each tool has typed input/output structs; the SDK derives the JSON Schema.
Input field names and semantics match the REST DTOs. A typed-error result (e.g.
"no MCP registered under that name") is returned as a tool error (`IsError`),
not a transport error.

### Transport and mounting

Use `mcp.NewStreamableHTTPHandler` and mount it on the control-plane Gin engine
under the `/admin` group at `admin_mcp.path` (default `/admin/mcp`), handling
the methods the streamable transport uses (`GET`, `POST`, `DELETE`). Because it
is under `/admin`, the ADR-0010 middleware authenticates and claim-gates the MCP
connection before the handler sees it — no separate auth for this surface.

New config section:

```yaml
admin_mcp:
  enabled: false          # default; true mounts the handler
  path: /admin/mcp        # must be under /admin/
```

When `enabled` is false (the default) nothing is mounted.

## Alternatives Considered

### Alternative A: MCP tools issue loopback HTTP calls to the Gin API

Advantages:
- Zero refactor of the handlers; guaranteed parity because it *is* the same
  code path.

Disadvantages:
- Every tool call is JSON-encoded, sent over a socket to localhost, routed,
  decoded, then the response re-encoded — pure overhead for an in-process call.
- The MCP server would need a base URL and a way to present an admin token to
  its own process, or a carve-out in the auth middleware for loopback — an
  auth-bypass shaped hole.
- Errors arrive as HTTP status codes to be re-mapped, losing the typed-error
  richness a shared Go API gives both adapters.

### Alternative B: MCP tools call the Registry / repositories / reloader directly

Advantages:
- No HTTP indirection.

Disadvantages:
- Duplicates the write-through ordering and validation currently inside the Gin
  handlers. Two copies of "register = Registry.Register then repo.Upsert then …"
  will drift. This is exactly the divergence the shared layer exists to prevent.

### Alternative C: A standalone `cmd/gateway-mcp` binary speaking MCP over stdio

Advantages:
- Natural fit for a local operator using an MCP-enabled editor / Claude Desktop.
- No HTTP surface, no token — process-level trust.

Disadvantages:
- It would need its own access to the persistence store and would run a *second*
  Registry / PolicyStore not shared with a running gateway — so its writes
  wouldn't be live on a running data plane (the whole point of the write-through
  in ADR-0005). It could only be an offline editing tool.
- More to build, package, and document for Phase 1.

Kept as possible future work; the shared `adminservice` layer makes it cheap to
add later against a running gateway's in-process state or a standalone store.

### Alternative D: A dedicated third listener/port for the MCP control server

Advantages:
- Clean separation from the REST admin API.

Disadvantages:
- A third listener to bind, TLS-terminate, firewall, and document, plus its own
  copy of the admin-auth wiring. Mounting under `/admin` on the existing
  control-plane listener reuses all of it.

## Decision Criteria

- **Behavioural parity with the REST API** — one implementation, two adapters.
- **Reuse** — the go-sdk is already vendored; the control-plane listener and
  ADR-0010 auth already exist.
- **Operational surface** — no new port, no new module, no new credential path.
- **Opt-in** — off by default.
- **Reversibility** — the `adminservice` extraction is valuable on its own; the
  MCP server is an isolated package that can be dropped without touching the
  REST API.

## Rationale

The risky part of "expose the admin API over MCP" is keeping the two surfaces
semantically identical. Extracting `adminservice` once and writing both the Gin
handlers and the MCP tools as thin adapters over it removes the possibility of
drift at the source, and the extraction improves the REST handlers regardless
(typed errors, testable application logic). Mounting the streamable-HTTP handler
under `/admin` means the MCP control server inherits authentication,
claim-gating, network placement, and TLS from decisions already made — the only
genuinely new thing is the tool layer itself.

## Consequences

### Positive

- A tool call and its REST equivalent are the same operation; parity is
  structural, not maintained by discipline.
- The MCP control server is authenticated and claim-gated for free (ADR-0010).
- The `adminservice` extraction makes the control-plane application logic unit-
  testable independent of Gin, and gives the REST handlers typed errors.
- Off by default; a deployment that doesn't want it is unaffected.
- No new module, listener, or credential mechanism.

### Negative

- The extraction touches every existing control-plane handler and its tests
  (contained: HTTP behaviour must be identical, guarded by the current tests).
- `gateway`'s `go.mod` gains a direct dependency on
  `github.com/modelcontextprotocol/go-sdk` (currently indirect).
- Another admin surface to document and reason about in the threat model, even
  though it shares the REST API's trust boundary.
- Two tool-schema definitions now exist for related shapes (the REST DTOs and
  the MCP tool structs); they must be kept in sync by review.

### Risks

- If `admin_mcp.enabled` is set but `admin_auth` is not, the MCP control server
  is exposed unauthenticated (same failure mode as the REST API without
  ADR-0010). Documented alongside the `admin_auth` "open + warn" default.
- The go-sdk's server API is pre-1.0-ish in ergonomics; a future major bump may
  require tool-registration changes. Isolated to the `adminmcp` package.

### Follow-up

- Optional stdio transport / `cmd/gateway-mcp` (Alternative C) if a local-operator
  workflow needs it.
- Audit-log admin actions (REST and MCP alike) against `auth.Claims.Subject` —
  Phase 3.

## Validation

- `adminservice`: unit tests with fake Registry / repositories / reloader for
  every service method, including the typed-error paths.
- `gateway/internal/controlplane`: the existing handler tests pass unchanged
  (HTTP contract preserved through the adapter rewrite).
- `adminmcp`: per-tool tests over the go-sdk in-memory transport — `tools/list`
  returns all tools with schemas; a `register_mcp` → `list_mcps` →
  `set_mcp_enabled` → `unregister_mcp` round-trip.
- `cmd/gateway/app`: `TestApp_AdminMCP_EndToEnd` — a go-sdk MCP client over
  streamable HTTP (with an admin JWT) against the fully wired gateway performs a
  register/list/disable/delete round-trip and an access-policy CRUD round-trip,
  with effects observable on both the data plane and the REST admin API; an
  unauthenticated MCP connection is rejected.

## References

- [ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md) — the
  registration operations these tools expose.
- [ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md) — the policy
  records these tools manage.
- [ADR-0005](0005-use-gin-for-control-plane-api.md) — the REST admin API mirrored
  1:1 and whose handlers are refactored into adapters here.
- [ADR-0010](0010-control-plane-admin-authentication.md) — the admin-auth gate
  the MCP control server is mounted behind.
- [api/admin-mcp.md](../../api/admin-mcp.md) — the tool catalog and usage.
- [CONFIG.md](../../CONFIG.md) — the `admin_mcp` config keys.
