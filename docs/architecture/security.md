# Security Architecture

This document describes the gateway's trust boundaries and the authentication
and authorization applied at each one. It is a map of *what is enforced where*;
the rationale for each mechanism lives in the referenced ADRs.

## Surfaces and trust boundaries

The gateway exposes two independent HTTP listeners
([overview](overview.md), [ADR-0001](decisions/0001-use-fasthttp-for-gateway-server.md),
[ADR-0005](decisions/0005-use-gin-for-control-plane-api.md)):

| Surface       | Listener                    | Callers                     | Trust boundary |
|---------------|-----------------------------|-----------------------------|----------------|
| Data plane    | `server.data_plane_addr`    | AI agents                   | Untrusted — every request is authenticated and authorized. |
| Control plane | `server.control_plane_addr` | Operators, the admin web UI, MCP admin clients | **Host-level** — see below. Authenticated + claim-gated when `admin_auth` is set; **must** still be network-isolated. |

Downstream MCP servers are treated as trusted infrastructure the operator
configured; the gateway is the client to them, not a server.

### Control-plane access is host code execution

Registering an MCP supplies the command and argv the gateway then runs
(`mcp.NewClient` → `exec.CommandContext`), so anyone who can call
`POST /admin/mcps` — or the equivalent `register_mcp` tool on `/admin/mcp` —
can execute an arbitrary program as the gateway process. That is the
feature, not a defect: launching stdio MCP servers is what the gateway does
([ADR-0003](decisions/0003-dynamic-mcp-registration-and-schema-discovery.md)).
Two consequences worth stating plainly:

- The subprocess starts *before* the gateway has verified that it speaks MCP,
  so a failed registration (`502 registration_failed`) means the program has
  already run. It is also bound to the gateway's lifetime, not the request's.
- `admin_auth` is a single "is an admin" gate with no per-operation RBAC, so
  there is no lower-trust admin tier. An operator trusted only to toggle
  `enabled` has the same host-level reach as any other.

Treat control-plane credentials as host credentials.

### The browser is inside the network perimeter

"Bind `control_plane_addr` to a trusted interface" is necessary but not
sufficient: a page an operator merely visits runs inside that perimeter and
can issue requests to `localhost`. Cross-origin requests escape preflight
only as *simple requests* — GET/HEAD/POST with `text/plain`,
`application/x-www-form-urlencoded` or `multipart/form-data` — and Gin's
`ShouldBindJSON` parses a body as JSON whatever its declared type, so such a
request would otherwise reach a write handler.

The control plane therefore refuses any `POST`/`PUT`/`PATCH` that does not
declare `Content-Type: application/json`, answering `415
unsupported_media_type` before authentication runs. That closes both halves
of the attack:

- A simple-request content type reaches the listener without a preflight and
  is refused on its media type, so no handler runs.
- `application/json` is not a simple content type, so the browser must
  preflight first — and the control plane answers no CORS headers at all, so
  the browser never sends the request.

`DELETE` and `GET` are exempt: they carry no body here, and a cross-origin
`DELETE` already requires a preflight of its own.

This is a structural defence, not a token: there is no CSRF token to manage
because the admin API is bearer-only and holds no ambient credentials — the
web UI sends `Authorization` from `sessionStorage`, never a cookie
([ADR-0014](decisions/0014-admin-ui-oidc-pkce-login.md)).

## Data-plane authentication & authorization

Per request to `POST /v1/call`
([data-plane API](../api/data-plane.md), [data lifecycle](data.md#request-lifecycle)):

1. **Authentication** — `auth.Validator` verifies the `Bearer` JWT's signature
   against the OIDC provider's JWKS (cached, background-refreshed) and checks
   `exp` / `iss` / `aud`
   ([ADR-0002](decisions/0002-jsonpath-regexp-claim-rule-engine.md)). Failure ⇒
   `401`.
2. **Authorization** — the unified policy engine evaluates the caller's decoded
   claims against `access_policies` (claim-match + `(mcp, tool)` grant)
   ([ADR-0004](decisions/0004-unified-policy-engine-for-access-and-filtering.md)).
   No matching grant ⇒ `403`, checked before resource existence so a missing
   grant never discloses whether an MCP/tool exists.
3. **Response filtering** — `filter_policies` strip fields from the tool
   response by claim-match ([ADR-0004]). This is data minimization, not access
   control.

## Control-plane authentication & authorization

Per request to `/admin/*` except `GET /admin/healthz`
([admin API](../api/admin.md#authentication),
[ADR-0010](decisions/0010-control-plane-admin-authentication.md)):

1. **Authentication** — the *same* `auth.Validator` and OIDC configuration as
   the data plane verify the `Bearer` JWT. There is no separate admin identity
   provider (a possible future `admin_auth.oidc` override is an ADR-0010
   follow-up). Failures ⇒ `401 missing_authorization` / `invalid_authorization`
   / `unauthorized`.
2. **Authorization** — the decoded claims are tested against `admin_auth.match`,
   a list of `{path, pattern}` claim rules (ADR-0002 syntax, ANDed) kept in the
   **config file, not the database**: the guard must not live inside the store
   it guards, and there must be no path for an admin API call to grant itself
   admin. Claims failing any rule ⇒ `403 forbidden`. A matcher error fails
   closed (`403`).
3. **Scope** — the gate covers MCP registration, access/filter policy CRUD, the
   Swagger UI, and (when enabled) the `/admin/mcp` MCP control server
   ([ADR-0011](decisions/0011-mcp-control-server.md)), which inherits it by
   being mounted under `/admin`. `GET /admin/healthz` is exempt for liveness
   probes.

### Backward-compatible default

If `admin_auth` is absent or `enabled = false`, the control plane is
**unauthenticated** and the gateway logs a prominent startup `WARN`. This
preserves existing deployments. Operators opt in by adding `admin_auth.match`;
the web UI signs in through the OIDC provider and attaches a token
([ADR-0014](decisions/0014-admin-ui-oidc-pkce-login.md)), so enabling the gate
no longer breaks it.

Until then, network isolation of `control_plane_addr` is the only *authentication*
control — and, per "The browser is inside the network perimeter" above, it is
not by itself enough. `config.example.toml` binds loopback for this reason.

## Known gaps (tracked, not defects)

- No audit trail of admin actions yet; the verified `auth.Claims.Subject` is
  stashed on the request context for the future Phase 3 audit log.
- No per-operation RBAC on the control plane — `admin_auth` is a single
  "is an admin" gate.
- No rate limiting or request-size limits on either surface
  ([data-plane API](../api/data-plane.md#current-limitations-tracked-not-bugs)).
- TLS termination for both listeners is still a `// TODO` in
  `config.ServerConfig`; run behind a TLS-terminating proxy.
- The control plane's "open + warn" default is not fail-closed; a future major
  version may flip it.
- MCP subprocesses inherit the gateway's environment, so a registered MCP can
  read the persistence DSN and any other secret in it
  ([#166](https://github.com/atsokha/mcplake/issues/166)).

## References

- [ADR-0002](decisions/0002-jsonpath-regexp-claim-rule-engine.md) — JWT claim-rule engine
- [ADR-0004](decisions/0004-unified-policy-engine-for-access-and-filtering.md) — unified access/filter policy engine
- [ADR-0005](decisions/0005-use-gin-for-control-plane-api.md) — control-plane surface
- [ADR-0010](decisions/0010-control-plane-admin-authentication.md) — control-plane admin authentication
- [ADR-0011](decisions/0011-mcp-control-server.md) — MCP control server (inherits the admin gate)
