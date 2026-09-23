# ADR-0003: Dynamic MCP Registration with Schema Discovery

- Status: Accepted
- Date: 2026-09-04

## Context

`docs/OVERVIEW.md` and `docs/SCAFFOLDING.md` describe MCP schema caching as a
one-time, startup-only step: the gateway connects to every MCP listed in
`config.toml` and fetches its tools once. That model does not support the
requirement to *register downstream MCPs* as an operation in its own right — the
design brief calls for the ability to add an MCP to a running gateway, at which point
its tools and request/response schemas are fetched, without requiring the operator to
restart the gateway or predict the full MCP set at deploy time.

Constraints:
- Access policies (ADR-0004) are defined in terms of MCP names and tool names, so an
  MCP must be fully known (name + tool list) before any policy can reference it
  meaningfully.
- The gateway must still support the existing startup-time config path
  (the `[[mcps]]` array in `config.toml`) — this is not being removed, only generalized.
- Air-gapped/operational simplicity: no external service registry; the gateway is the
  only place registration state lives.

## Decision

Introduce a single internal registration operation,
`Registry.Register(reg MCPRegistration) error`, used by both:

1. **Startup path** — for each `[[mcps]]` entry in `config.toml`, the gateway calls
   `Registry.Register` during boot, before the HTTP server starts accepting traffic.
2. **Runtime path** — an admin endpoint (`POST /admin/mcps`) accepts the same
   `MCPRegistration` shape and calls the identical `Registry.Register`.

`Register` performs, synchronously, before returning success to the caller:

```
connect     -> mcp.NewClient(reg.Transport, reg.Connect)
discover    -> client.ListTools(ctx)         // MCP tools/list
cache       -> store {name, inputSchema, outputSchema} per tool
activate    -> reg.Status = "active"; make visible to Policy Engine and Router
```

If any step fails, the MCP is stored with `Status = "unreachable"` (startup path:
logged as a warning, gateway still starts — one bad MCP must not block the others;
runtime path: the admin call returns an error and nothing is made visible to the
Policy Engine). A corresponding `Registry.Unregister(name)` closes the client and
removes it from the cache and from consideration by the Policy Engine; in-flight
calls to that MCP are allowed to finish, new calls are rejected with 404.

Re-registering an existing name is treated as an update: the old client is closed
after the new one is confirmed connected (make-before-break), and the tool/schema
cache is replaced wholesale, not merged — this keeps "what tools does this MCP have
right now" unambiguous instead of accumulating stale entries across schema changes.

## Alternatives Considered

### Alternative A: Startup-only registration (status quo)

Advantages:
- Simplest possible model; no admin API, no runtime state transitions to reason
  about.

Disadvantages:
- Does not meet the explicit requirement to register MCPs as a first-class operation;
  adding an MCP requires a config change and restart, which is precisely what this
  milestone is meant to remove.

### Alternative B: Single `Register` op for both static and dynamic MCPs (chosen)

Advantages:
- One code path to test and reason about; static config becomes "registration calls
  made at boot" rather than a parallel mechanism.
- Matches the requirement directly: register → fetch tools/schemas → available for
  policy.

Disadvantages:
- Requires defining failure semantics for partial startup (some MCPs reachable, some
  not) that a purely static model wouldn't need to think about.

### Alternative C: External service registry / discovery (e.g. Consul)

Advantages:
- Decouples registration from the gateway process; supports multi-gateway fleets
  sharing one MCP inventory.

Disadvantages:
- A new external dependency and a new failure mode (registry unavailable), directly
  against the project's air-gapped, single-binary deployment goal. Multi-gateway
  federation is an explicit non-goal for this milestone
  ([overview.md](../overview.md#non-goals-for-this-milestone)).

## Decision Criteria

- Meets the explicit "register downstream MCPs, then fetch tools/schemas" requirement.
- Operational simplicity / fits single-binary, air-gapped deployment.
- Failure isolation (one bad MCP should not take down the gateway or block others).
- Consistency between config-driven and runtime-driven registration.

## Rationale

A single internal `Register` operation used by both the config loader and the admin
API is the smallest change that satisfies the requirement, keeps static and dynamic
MCPs behaviorally identical (no operator surprise about which path they used), and
adds no external dependency, consistent with the project's deployment goals.

## Consequences

### Positive

- Operators can add an MCP to a running gateway without a restart.
- Access/filter policies can be authored against MCPs that didn't exist at gateway
  boot time.
- One code path means one set of tests for registration correctness.

### Negative

- The gateway now has runtime-mutable state (the registry) protected by a
  `sync.RWMutex`, where previously schema caching was populate-once-at-startup and
  effectively read-only for the process lifetime.
- The admin API itself needs its own authorization story (who is allowed to register
  an MCP) — tracked as follow-up, not solved by this ADR.

### Risks

- Concurrent tool calls during a re-registration (schema swap) must not observe a
  half-updated tool list; mitigated by the make-before-break replace described above,
  swapping the cached tool map atomically under the registry lock.
- An MCP that lies about its own schema (or changes behavior without changing its
  advertised schema) is out of scope — the gateway trusts `tools/list` output as
  given.

### Follow-up

- Define admin API authentication/authorization (likely: a separate, more privileged
  claim rule / policy, reusing ADR-0002's engine, gated on a dedicated
  "gateway-admin" claim) before exposing `/admin/mcps` outside a trusted network.
- Decide whether unreachable MCPs are retried automatically (e.g. periodic
  reconnect) or require an explicit re-`Register` call — deferred until the failure
  mode is observed in practice.
- ~~The one-time-at-registration schema fetch goes stale if a running MCP adds or
  removes a tool.~~ Addressed by
  [ADR-0013](0013-periodic-mcp-schema-refresh.md): an optional
  `mcp.schema_refresh_interval` re-runs `tools/list` on the live client on a
  timer and swaps the cached schema in, without reconnecting.
- ~~Only the stdio transport is implemented, so every downstream MCP must be a
  subprocess of the gateway.~~ Addressed by
  [ADR-0017](0017-http-and-sse-transports-for-downstream-mcps.md): `http`
  (Streamable HTTP) and `sse` join `stdio`, so an MCP can run in its own
  container, pod or host. Registration, discovery and the make-before-break
  swap described above are unchanged — only how the client is dialed differs.

## Validation

Integration test: register two MCPs (one via config at boot, one via the admin
endpoint at runtime), confirm both appear identically in the registry and both are
usable by an access policy. Failure test: register an MCP with an unreachable
command/URL, confirm the gateway still starts and other MCPs remain usable.

## References

- [data.md](../data.md#mcp-registration)
- [components.md](../components.md#mcp-registry-cache-extended)
