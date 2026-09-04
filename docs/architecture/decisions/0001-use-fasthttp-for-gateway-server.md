# ADR-0001: Use fasthttp for the Gateway Data-Plane HTTP Server

- Status: Accepted
- Date: 2026-09-04
- Scope note (2026-09-04): This ADR covers the **data plane** only — the tool-call
  proxy path described below. The control-plane admin API (CRUD for policies and MCP
  registrations) is a separate HTTP surface built on Gin; see
  [ADR-0005](0005-use-gin-for-control-plane-api.md) for why that surface intentionally
  does not reuse this decision.

## Context

The gateway sits on the hot path of every tool call between an AI agent and a
downstream MCP. The project's stated performance target (`README.md`) is ~1-2ms of
gateway overhead per request, and the gateway is meant to run as a single, dependency-light
binary suitable for air-gapped deployment.

The current scaffold (`gateway/internal/gateway.go`) uses `net/http.Server` as a
placeholder. We need to decide the concrete HTTP server technology before implementing
`Gateway.Start` and the tool-call handler, since this affects the handler signature
used throughout `gateway/internal`. (Admin/registration handlers are addressed by
[ADR-0005](0005-use-gin-for-control-plane-api.md), not here.)

Requirements:

- Low per-request allocation/latency overhead — every request pays JWT parsing +
  JSONPath evaluation + regexp matching + a downstream call, on top of whatever the
  HTTP layer itself costs.
- HTTP/1.1 is sufficient; downstream MCP transports are stdio/SSE/HTTP, not HTTP/2
  server push or gRPC.
- Minimal external dependencies, consistent with the project's existing bias
  (`docs/SCAFFOLDING.md`) toward stdlib-first modules.
- Must support streaming SSE-style responses for MCP transports that use SSE, and
  must not block the gateway's own event loop on a slow downstream MCP.

## Decision

Use [`valyala/fasthttp`](https://github.com/valyala/fasthttp) directly as the
data-plane HTTP server, replacing the placeholder `net/http.Server` in
`gateway/internal/gateway.go`, and bind it to its own listener/port, separate from the
Gin-based control-plane API of ADR-0005. Routing is handled with a small manual
method+path switch inside the `fasthttp.RequestHandler`, not a separate router
dependency (the data plane's route count for this milestone — one tool-call endpoint,
one health endpoint — does not justify adding `fasthttp/router` or a framework like
Fiber on top of it).

Because `fasthttp.RequestCtx` is pooled and reused immediately after the handler
returns, any data needed beyond the handler's synchronous execution (claims, request
body, target MCP/tool) is copied out of the `RequestCtx` before starting the
downstream MCP call, and each MCP call runs under its own `context.Context` with an
explicit timeout rather than borrowing the request's lifecycle.

## Alternatives Considered

### Alternative A: `net/http` (stdlib)

Advantages:
- Zero extra dependency; idiomatic `context.Context` support throughout; every other
  Go HTTP tool (middleware, tracing, pprof) works out of the box.
- Simpler mental model — no pooled-object caveats.

Disadvantages:
- Materially higher allocations per request (one `*http.Request`/`*http.Response`
  pair, header maps, etc.), which cuts directly against the stated ~1-2ms overhead
  target once JWT + JSONPath + regexp evaluation are added on top.

### Alternative B: `fasthttp` (chosen)

Advantages:
- Substantially lower allocation and latency overhead than `net/http` under load,
  by design (object pooling, zero-copy where possible).
- Still a single dependency, no framework lock-in.

Disadvantages:
- Does not implement `net/http.Handler`/`http.ResponseWriter` — any middleware or
  library written against stdlib HTTP interfaces needs an adapter or a rewrite.
- `RequestCtx` is pooled/reused after the handler returns — requires discipline not to
  retain it across goroutines, which is an easy mistake for contributors unfamiliar
  with fasthttp.
- No native HTTP/2 support (not a requirement here, but forecloses it later without a
  bigger change).

### Alternative C: A framework built on fasthttp (e.g. Fiber)

Advantages:
- fasthttp's performance plus batteries-included routing, middleware, and a more
  ergonomic API.

Disadvantages:
- Adds a second dependency and its own idioms/versioning on top of fasthttp, for
  convenience the gateway doesn't need yet given its small, fixed route set.
- Larger surface to audit for an air-gapped/security-sensitive deployment target.

## Decision Criteria

- Performance (primary — this is the reason to move off `net/http` at all).
- Operational simplicity / dependency footprint (project value: single binary, minimal deps).
- Developer experience for contributors (weighed against, but not overriding, performance).

## Rationale

Performance is the dominant criterion given the project's explicit low-overhead goal
and the fact that every request already pays for JWT validation plus two rounds of
JSONPath/regexp policy evaluation. `fasthttp` gives the largest available reduction in
per-request overhead for a single added dependency. A full framework (Fiber) buys
convenience the gateway's small route table doesn't need, at the cost of an extra
dependency layer; raw `fasthttp` with a manual switch keeps the dependency count the
same as choosing `net/http` while getting the performance benefit.

## Consequences

### Positive

- Lower, more predictable per-request latency, leaving more of the 1-2ms budget for
  policy evaluation and the downstream call.
- Single additional dependency, no framework abstraction to learn beyond fasthttp
  itself.

### Negative

- Gateway HTTP handlers cannot use stdlib `net/http` middleware directly; any needed
  cross-cutting behavior (logging, panic recovery, metrics) must be written or adapted
  for fasthttp.
- Contributors must understand the `RequestCtx` pooling model to avoid subtle bugs
  (use-after-reuse of request data in goroutines).

### Risks

- If a future requirement needs HTTP/2 or WebSocket support not well covered by
  fasthttp, this decision would need revisiting.

### Follow-up

- Document the "copy out of `RequestCtx` before going async" rule in
  `CONTRIBUTING.md` once the gateway handler is implemented, so it isn't only in this
  ADR.

## Validation

Benchmark the tool-call handler (auth + policy evaluation + a no-op mock MCP call)
under load once implemented, and confirm p50/p99 gateway overhead against the
~1-2ms target before closing out this milestone.

## References

- [`README.md`](../../../README.md) — states the ~1-2ms overhead target.
- [`docs/SCAFFOLDING.md`](../../SCAFFOLDING.md) — minimal-dependency guidance.
- [components.md](../components.md#gateway-server-gateway)
- [ADR-0005](0005-use-gin-for-control-plane-api.md) — why the control plane deliberately
  uses a different HTTP stack than this one.
