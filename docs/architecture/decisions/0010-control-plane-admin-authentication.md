# ADR-0010: JWT Authentication for the Control-Plane Admin API

- Status: Accepted
- Date: 2026-09-10

## Context

[ADR-0005](0005-use-gin-for-control-plane-api.md) shipped the Gin control-plane
(`/admin/*`: MCP registration, access/filter policy CRUD, the embedded web UI,
and the Swagger UI) with **no authentication or authorization**. Its only
protection is the operational requirement to bind `server.control_plane_addr` to
a trusted network/interface. ADR-0005's follow-up records the intent to design
this "likely a dedicated admin claim/policy, reusing the ADR-0002 claim-rule
engine" before exposing `/admin/*` beyond localhost.

Requirements:

- An admin caller must prove identity with a bearer token, verified the same way
  the data plane verifies agent tokens (signature against the OIDC provider's
  JWKS, plus `exp`/`iss`/`aud`) — the deployment already runs an OIDC provider
  for the data plane ([ADR-0002](0002-jsonpath-regexp-claim-rule-engine.md)).
- "Who is an admin" must be **operator-controlled configuration in the
  config file, not a database table**. The control-plane's own persistence
  ([ADR-0006](0006-gorm-sqlite-postgres-persistence.md)) stores the objects the
  admin API *manages*; letting that same store also decide who may call the API
  is a bootstrapping and blast-radius problem (a compromised admin call could
  grant itself admin). Config-file rules are external to the running system and
  changed only by someone with host access.
- The rule language should be the one already in the codebase — the ADR-0002
  JSONPath + regexp claim rules (`router.ClaimRule` / `router.ClaimMatcher`),
  which `config.Config.Validate()` already compiles for access/filter policies.
- Backward compatibility: existing deployments (and the embedded web UI, which
  today calls `/admin/*` with no credentials) must not break on upgrade.
- Non-goal: per-operation RBAC. This is a single "is an admin" gate. Finer
  authorization, an audit trail of admin actions, and a distinct admin identity
  provider are future work.

## Decision

Add a Gin middleware on the `/admin` route group that authenticates and
authorizes every request except `GET /admin/healthz`.

**Authentication** reuses `auth.Validator` — the *same* instance the data plane
uses, constructed from the existing `oidc:` config (`jwks_url`, `issuer`,
`audience`). The middleware extracts a `Bearer` token from the `Authorization`
header and calls `ValidateToken`.

**Authorization** is a new `admin_auth` config section:

```toml
# [admin_auth]
# enabled = true           # optional, default true

[[admin_auth.match]]        # ADR-0002 {path, pattern} rules, ANDed
path = "$.role"
pattern = "^admin$"

[[admin_auth.match]]
path = "$.iss"
pattern = "^https://auth\\.example\\.com$"
```

`match` is loaded into a `router.ClaimMatcher` and evaluated against the decoded
claim set (`auth.Claims.Raw`). All rules must match (AND), consistent with
access/filter policy `match` semantics. `config.Config.Validate()` compiles
every rule at startup via the existing `validateClaimRules` helper, so a
malformed `path`/`pattern` fails at load time, not per request.

**Error responses** reuse the data-plane `{"error": "<code>", "message": "..."}`
shape:

| Status | `error` code             | When                                                    |
|--------|--------------------------|---------------------------------------------------------|
| 401    | `missing_authorization`  | No `Authorization` header.                              |
| 401    | `invalid_authorization`  | Header present but not a non-empty `Bearer` token.      |
| 401    | `unauthorized`           | Token fails signature/`exp`/`iss`/`aud` validation.     |
| 403    | `forbidden`              | Token valid but claims don't satisfy `admin_auth.match`.|

**Backward compatibility / rollout.** When `admin_auth` is absent or
`enabled = false`, the middleware is not installed: `/admin/*` stays open exactly
as today, and the gateway logs a single prominent `WARN` at startup
("control-plane admin API is UNAUTHENTICATED — set admin_auth.match or bind
control_plane_addr to a trusted interface"). This keeps existing deployments and
the current embedded web UI working; operators opt in by adding `admin_auth.match`.

**Scope of the gate.** Everything under `/admin` — including `/admin/swagger/*`
and (once it exists, [ADR-0011](0011-mcp-control-server.md)) `/admin/mcp` — sits
behind the middleware. `GET /admin/healthz` is explicitly exempt so liveness
probes need no credentials. The web UI's own static assets are served from the
engine root (`/`), not `/admin`, so they remain reachable; the UI's *API calls*
to `/admin/*` will need a token — wiring that into the SPA is tracked under the
Admin UI epic (#76), not here.

## Alternatives Considered

### Alternative A: Static shared secret (bearer token or HTTP Basic) in config

Advantages:
- Trivial to implement; no OIDC dependency for the control plane.
- No JWKS round-trip.

Disadvantages:
- A second, unrelated credential system to distribute, rotate, and revoke, when
  the deployment already has an OIDC provider and JWT tooling.
- No identity — every admin is indistinguishable, making a future audit trail
  (Phase 3) impossible to attribute.
- Secret sprawl: the same string in every operator's shell history / CI config.

### Alternative B: Mutual TLS on the control-plane listener

Advantages:
- Strong, transport-level; no application code in the request path.
- Naturally client-identifying via the client cert.

Disadvantages:
- Certificate issuance/rotation is a heavier operational burden than issuing a
  JWT from the OIDC provider the deployment already runs.
- TLS config for the two listeners is still a `// TODO` in `config.ServerConfig`;
  this would block admin auth on that unrelated work.
- Doesn't compose with the browser-based web UI without client-cert prompts.

### Alternative C: Database-backed admin RBAC (admins/roles tables + admin API to manage them)

Advantages:
- Runtime-editable without host access; supports fine-grained per-operation
  permissions later.

Disadvantages:
- Bootstrapping: the first admin has to come from *somewhere* (config or a seed),
  so a config path is needed regardless.
- Blast radius: an attacker with any admin call can escalate/persist by writing
  the admin tables — the thing guarding the API lives inside the API's own
  writable store. The requirement is explicitly "rules in the config file, not
  the database".
- Much larger surface (schema, migrations, CRUD, its own tests) for a gate that
  Phase 1 needs to be a simple boolean.

### Alternative D: A dedicated admin OIDC provider / audience distinct from the data plane

Advantages:
- Cleaner separation; admin tokens can't be replayed against the data plane and
  vice versa.

Disadvantages:
- A second JWKS URL / issuer / audience to configure and operate for a
  single-instance air-gapped deployment that today runs one provider.
- The `admin_auth.match` rules already let an operator require, e.g., a specific
  `aud` or a group claim that only admin tokens carry — most of the isolation
  benefit without the second provider.

Kept as a follow-up: an optional `admin_auth.oidc` block overriding
`jwks_url`/`issuer`/`audience` for the admin surface, added only if a deployment
actually needs it.

## Decision Criteria

- **Reuse over new infrastructure** — the deployment already has OIDC + JWKS +
  the ADR-0002 claim-rule engine; admin auth should be those parts recombined.
- **Config-file source of truth for "who is an admin"** — external to the running
  system, no self-escalation path.
- **Backward compatibility** — no forced break for existing deployments or the
  current web UI on upgrade.
- **Minimal surface for Phase 1** — a boolean gate, not an RBAC subsystem.
- **Operability** — one credential system, one place to look when a call is
  rejected (structured error codes matching the data plane).

## Rationale

Authentication and the "is an admin" decision are two problems the codebase
already has answers for: `auth.Validator` verifies JWTs, and
`router.ClaimMatcher` evaluates JSONPath+regexp claim rules that
`config.Validate()` already compiles. Composing them in a Gin middleware is the
smallest change that satisfies the requirement, adds no new credential system,
and produces an identity that a later audit trail can attribute. Keeping the
rules in `config.toml` rather than the database is a deliberate blast-radius
choice: the guard must not live inside the store it guards. The "absent config ⇒
open + warning" default trades a strict-by-default posture for a non-breaking
upgrade, which is acceptable because ADR-0005 already documents binding the
control plane to a trusted network as the operational baseline.

## Consequences

### Positive

- No new credential system: admin tokens are issued by the OIDC provider the
  deployment already runs, verified by the code path the data plane already uses.
- "Who is an admin" is operator-controlled config, external to the running
  system — no self-escalation via the admin API.
- Structured, data-plane-consistent error codes make a rejected admin call
  self-diagnosing.
- `/admin/mcp` (ADR-0011) inherits this gate for free by mounting under `/admin`.
- Zero behaviour change on upgrade for deployments that don't set `admin_auth`.

### Negative

- The "absent ⇒ open + warning" default means an operator who never reads the
  warning stays unauthenticated. Mitigated by the prominence of the log line and
  by `docs/CONFIG.md` / `docs/api/admin.md` guidance; a future major version may
  flip the default to fail-closed.
- `/admin/swagger/*` now requires a token, so the interactive API explorer is no
  longer anonymously reachable.
- The embedded web UI's `/admin/*` calls will 401 once `admin_auth` is set until
  the SPA is taught to attach a token (#76).
- Admin auth is coupled to the data plane's OIDC config until the optional
  `admin_auth.oidc` follow-up lands.

### Risks

- If an operator sets `admin_auth.match` to an over-broad rule (e.g. `$.iss`
  present), every authenticated data-plane user becomes an admin. Documented:
  `match` should pin a claim only admin tokens carry.
- A permissive regexp (missing anchors) is a classic footgun — `^...$` anchoring
  is shown in every doc example, and rules still compile-check at startup.

### Follow-up

- Optional `admin_auth.oidc` block to point the admin surface at a different
  provider/audience than the data plane (Alternative D).
- Teach the embedded web UI to obtain and attach an admin token (#76).
- Audit trail of admin actions attributed to `auth.Claims.Subject` — folds into
  the Phase 3 audit-log work, not this change.
- Consider flipping the default to fail-closed in a future major version.

## Validation

- `auth`: unchanged — reused as-is.
- `config`: `admin_auth` present / absent / `enabled = false` / malformed `match`
  rule parsing and compilation via `Config.Validate()`.
- `gateway/internal/controlplane`: middleware handler tests for all four
  rejection paths and the pass-through path; `/admin/healthz` reachable with no
  token.
- `cmd/gateway/app`: `TestApp_AdminAuth_EndToEnd` — the fully wired gateway with a
  local test JWKS and an `admin_auth.match` rule: admin-claim token ⇒ `200` on
  `/admin/mcps`, no token ⇒ `401`, valid token failing the rule ⇒ `403`,
  `/admin/healthz` open. Plus a test asserting the startup WARN when `admin_auth`
  is absent.

## References

- [ADR-0005](0005-use-gin-for-control-plane-api.md) — the control-plane surface
  this ADR secures; its authN/authZ follow-up is resolved here.
- [ADR-0002](0002-jsonpath-regexp-claim-rule-engine.md) — the `{path, pattern}`
  claim-rule engine `admin_auth.match` reuses.
- [ADR-0011](0011-mcp-control-server.md) — mounts `/admin/mcp` behind this gate.
- [api/admin.md](../../api/admin.md) — the `admin_auth` section and per-endpoint
  401/403 responses.
- [CONFIG.md](../../CONFIG.md) — the `admin_auth` config keys.
