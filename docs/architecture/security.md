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
| Control plane | `server.control_plane_addr` | Operators, the admin web UI, MCP admin clients | Semi-trusted — authenticated + claim-gated when `admin_auth` is set; **must** still be network-isolated. |

Downstream MCP servers are treated as trusted infrastructure the operator
configured; the gateway is the client to them, not a server.

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
preserves existing deployments and the current web UI (which does not yet send a
token — [#76](https://github.com/atsokha/mcplake/issues/76)). Operators opt in
by adding `admin_auth.match`. Until then, network isolation of
`control_plane_addr` is the only control — as it was before ADR-0010.

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

## References

- [ADR-0002](decisions/0002-jsonpath-regexp-claim-rule-engine.md) — JWT claim-rule engine
- [ADR-0004](decisions/0004-unified-policy-engine-for-access-and-filtering.md) — unified access/filter policy engine
- [ADR-0005](decisions/0005-use-gin-for-control-plane-api.md) — control-plane surface
- [ADR-0010](decisions/0010-control-plane-admin-authentication.md) — control-plane admin authentication
- [ADR-0011](decisions/0011-mcp-control-server.md) — MCP control server (inherits the admin gate)
