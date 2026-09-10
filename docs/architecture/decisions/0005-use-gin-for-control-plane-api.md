# ADR-0005: Use Gin for the Control-Plane Admin API

- Status: Accepted
- Date: 2026-09-04

## Context

[ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md) and
[ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md) require an admin
surface for CRUD-style operations: registering/unregistering downstream MCPs, and
creating/updating/deleting `AccessPolicy` and `FilterPolicy` records. This is a
distinct workload from the tool-call data plane:

- Low request volume (operators and the future Admin/Graph UI, not every agent tool
  call), so the ~1-2ms-overhead pressure that motivated
  [ADR-0001](0001-use-fasthttp-for-gateway-server.md) does not apply here.
- CRUD-shaped: request binding/validation, path/query parameters, JSON
  (de)serialization of richer structures (nested `ClaimRule` lists, `Grant` lists),
  and conventional REST semantics — exactly what a batteries-included web framework is
  for.
- Needs to be easy for contributors to extend as new admin endpoints are added
  (list MCPs, view a policy, dry-run a claim match against a sample JWT for
  debugging), which favors ecosystem maturity and developer familiarity over raw
  throughput.
- Will sit in front of the persistence layer
  ([ADR-0006](0006-gorm-sqlite-postgres-persistence.md)), so it benefits from
  middleware for things like request-scoped DB transactions and structured error
  responses.

We need to decide whether the control plane reuses the fasthttp stack from ADR-0001,
or uses a separate, ergonomics-first framework.

## Decision

Build the control-plane admin API on [Gin](https://github.com/gin-gonic/gin), run as
a second HTTP surface (its own listener/port) alongside the fasthttp data plane. Gin
is `net/http`-based, so it can use standard `net/http` middleware, testing helpers
(`httptest`), and any future library written against `http.Handler` without an
adapter.

Scope of this surface: `/admin/mcps` (register/unregister/list), `/admin/access-policies`,
`/admin/filter-policies` (CRUD), and a health/readiness endpoint for the control
plane itself. Tool calls never go through Gin; they stay on the fasthttp data plane
per ADR-0001.

## Alternatives Considered

### Alternative A: Extend the fasthttp mux from ADR-0001 to also serve admin routes

Advantages:
- One HTTP stack, one listener, one dependency — simplest possible topology.

Disadvantages:
- fasthttp has no built-in request binding/validation; every admin endpoint would
  hand-roll JSON decoding and field validation for `AccessPolicy`/`FilterPolicy`/
  `MCPRegistration` payloads, which are nested and richer than the data plane's
  simple tool-call envelope.
- Reintroduces the `RequestCtx`-pooling caveats from ADR-0001 for a workload
  (CRUD, low QPS) that gets no benefit from fasthttp's performance characteristics.
- Standard `net/http`-based tooling (validation libraries, ORM request helpers) can't
  be used directly.

### Alternative B: `net/http` + a lightweight router (e.g. `chi`) instead of Gin

Advantages:
- Smaller dependency than Gin; closer to stdlib idioms.

Disadvantages:
- Gin was explicitly requested and is more widely adopted, meaning more contributors
  will already be familiar with it and more examples/middleware exist for the CRUD
  patterns this surface needs (binding, validation, grouped routes).

### Alternative C: Gin for the control plane (chosen)

Advantages:
- Built-in request binding and validation (`ShouldBindJSON` + struct tags) fits the
  CRUD shape of the admin API directly.
- Full `net/http` compatibility — works with standard middleware, `httptest`, and any
  library targeting `http.Handler`.
- Widely adopted, well-documented, large middleware ecosystem — lowest ramp-up cost
  for contributors, which matters more here than on the performance-critical data
  plane.

Disadvantages:
- A second HTTP framework in the codebase (fasthttp for data plane, Gin for control
  plane), rather than one — see Consequences.
- Gin's overhead is irrelevant at admin-API volumes but is a real cost that would
  matter if this surface were ever repurposed for the data plane; it should not be.

## Decision Criteria

- Developer ergonomics for CRUD endpoints (binding, validation, routing).
- Ecosystem maturity / contributor familiarity.
- Compatibility with standard `net/http` middleware and tooling.
- Explicit non-goal: raw throughput (this is not the data plane).

## Rationale

The control plane and data plane have opposite priorities: the data plane is
performance-critical and narrow (ADR-0001), the control plane is CRUD-shaped,
low-volume, and benefits from ecosystem maturity and ergonomics. Using Gin here
instead of stretching fasthttp to cover both means each surface is built with the
tool suited to its actual workload, rather than compromising the data plane's
performance story or hand-rolling CRUD scaffolding on fasthttp.

## Consequences

### Positive

- Admin endpoints get request binding/validation, grouped routing, and middleware
  for free, reducing boilerplate for `AccessPolicy`/`FilterPolicy`/`MCPRegistration`
  CRUD.
- Full compatibility with `net/http`-ecosystem tooling (logging middleware, `httptest`
  for handler tests, future auth middleware).
- Clear separation of concerns: performance-sensitive code stays isolated to the
  fasthttp path; nobody is tempted to add CRUD convenience helpers to the data-plane
  handler.

### Negative

- Two HTTP frameworks in one codebase (fasthttp + Gin) instead of one, meaning two
  sets of idioms/middleware patterns for contributors to learn.
- Two listeners/ports to operate and document (data plane vs. control plane),
  including separate TLS/auth configuration for each.
- The admin API itself needs its own authentication/authorization story
  (tracked as follow-up in [ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md#follow-up));
  Gin's middleware chain is where that will be implemented once designed.

### Risks

- If the two surfaces are deployed without clear network separation (e.g. both
  exposed publicly), the control plane becomes an attack surface for policy
  tampering. Documented as an operational requirement: the control-plane port should
  be bound to a trusted network/interface by default.

### Follow-up

- ~~Design control-plane authN/authZ (likely a dedicated admin claim/policy, reusing
  the ADR-0002 claim-rule engine) before exposing `/admin/*` beyond localhost/trusted
  networks.~~ Resolved by [ADR-0010](0010-control-plane-admin-authentication.md): a
  Gin middleware reusing `auth.Validator` for JWT verification plus an
  `admin_auth.match` claim-rule list (ADR-0002 syntax) from `config.toml`.
- Document the two-port topology (data-plane port, control-plane port) in
  `config.example.toml` and `docs/CONFIG.md` once implemented.

## Validation

Handler tests using `net/http/httptest` for each admin endpoint (register/unregister
MCP, CRUD for both policy types), plus an integration test confirming a policy
created via the Gin admin API is immediately visible to the fasthttp data-plane's
`Authorize`/`FieldsToRemove` calls (through the shared persistence layer in
[ADR-0006](0006-gorm-sqlite-postgres-persistence.md)).

## References

- [ADR-0001](0001-use-fasthttp-for-gateway-server.md) — the data-plane counterpart
  this ADR deliberately does not reuse.
- [ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md) — the
  registration operations this API exposes.
- [ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md) — the policy
  records this API manages.
- [ADR-0006](0006-gorm-sqlite-postgres-persistence.md) — where these records are
  stored.
- [components.md](../components.md#control-plane-api-gin)
