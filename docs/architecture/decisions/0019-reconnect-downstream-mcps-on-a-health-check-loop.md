# ADR-0019: Reconnect Downstream MCPs on a Health-Check Loop

- Status: Accepted
- Date: 2026-09-23

## Context

[ADR-0017](0017-http-and-sse-transports-for-downstream-mcps.md) added the `http`
and `sse` client transports, and they work: an MCP registered over either one
connects, discovers its tools, and is callable through the data plane with
policies applying exactly as they do for a stdio MCP.

What did not come with them is the *session lifecycle* a networked downstream
implies. Registration built a session once and the gateway held it forever. That
was a reasonable model while stdio was the only transport, because a stdio
downstream is the gateway's own child process: started by the gateway, living
exactly as long as it, and never restarted out from under a live session. For an
MCP reached over the network none of that holds. Two failure modes followed, both
reproduced against real MCP servers (the TypeScript SDK's
`@modelcontextprotocol/server-everything` and a Python-SDK FastMCP server) over
both transports:

**A downstream restart was unrecoverable.** Stop and start the server and every
subsequent call returned `502 upstream_error`, permanently. Worse, the registry
went on reporting the endpoint `active` with its full tool list — a green status
in the admin UI on something that could not serve a single call. The only
recoveries were restarting the gateway or re-`POST`ing the registration by hand.

**A failed connect was never retried.** An MCP registered before its server was
listening was filed as `unreachable` with zero tools, and stayed there
indefinitely after the server arrived.

Neither is exotic. `compose.yaml` gives the demo's `postgres-mcp`
`restart: on-failure`, so the first time it restarted the demo was bricked. A pod
reschedule, a redeploy, or a network blip did the same thing.

Nothing in the codebase was capable of rebuilding a client. `cache.RegisterAll`
logs a failure and carries on, which is [ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md)'s
deliberate tolerance of a partially-available set of MCPs at startup. And
`Registry.RefreshActive` ([ADR-0013](0013-periodic-mcp-schema-refresh.md)) only
visits registrations that are *already* `active`, and calls `tools/list` on their
*existing* client — by construction it cannot notice a dead session, let alone
replace one.

`deploy/demo/entrypoint.sh` already compensates for half of the second symptom
with a TCP wait before exec'ing the gateway. That is a demo working around a gap
in the gateway; a real deployment has no such hook.

## Decision

**Run a background health-check loop that pings every registered MCP and
reconnects the ones that have stopped answering.** It is on by default.

Per tick, for each registration:

- **`active` with a live session** — `ping` answers, nothing happens. This is the
  overwhelmingly common case and costs one empty round trip per MCP.
- **`active` with a dead session** — the ping fails and a reconnect is attempted
  immediately. On success the registration never leaves `active`: the client and
  tools are swapped underneath it, so a concurrent caller sees the old working
  client or the new one, never a gap.
- **not `active`** — a reconnect is attempted, which is how a registration that
  failed at startup becomes usable once its server arrives.
- **reconnect failed** — the registration is demoted to `unreachable` and its dead
  client closed, with capped exponential backoff so a permanently misconfigured
  endpoint is not retried every tick.

This applies to stdio too. A crashed subprocess is the same defect, and a uniform
rule is easier to operate than a per-transport one; the only difference is that
reconnecting a stdio MCP respawns the subprocess.

### A failed ping is retried once before anything is rebuilt

One failed ping does not mean the session is gone. The common cause, observed
immediately once the loop was running against a real uvicorn-hosted MCP, is a
*pooled TCP connection* the server closed and the client had not noticed: the
shared http transport holds an idle connection for 90 seconds
(`mcp.httpTransportClient`), while a server behind uvicorn or Node closes one
after five, so the first request after a quiet spell can take a corpse out of
the pool and fail with `EOF`. The MCP session is fine; only the socket died.

A second ping dials afresh — Go's transport evicts the dead connection on the
error — and carries the same session id, so it succeeds, where a server that
has genuinely restarted fails again. One extra round trip on the failure path
buys not throwing away a working session, and not re-running `tools/list` to
rebuild what was already there. Against a uvicorn MCP this turned roughly one
spurious reconnect every three minutes into none.

### `ping`, not `tools/list`

Liveness and schema freshness are separate questions with separate costs.
`ping` is an empty round trip; `tools/list` may page through a large catalogue,
and is already ADR-0013's job on its own interval. Reusing the refresher's call
would have meant paying schema-discovery cost at health-check frequency, or
checking health only as often as it is worth re-reading schemas.

### A separate loop, on by default

The health loop is deliberately *not* folded into ADR-0013's `SchemaRefresher`,
and it defaults on where the refresher defaults off.

Stale schemas are a correctness nicety with a real per-tick cost, so making an
operator ask for them is right. "The gateway recovers when a downstream
restarts" is not in that category — it is the difference between a working
gateway and one that fails permanently on a routine event, and no operator
should have to discover a config key to get it. `mcp.health_check_interval`
therefore defaults to 30 seconds and is disabled only by an explicit `"0"`
(which is why, unlike `schema_refresh_interval`, it is a pointer in the config
struct: an omitted key and a zero have to mean different things).

Thirty seconds bounds the worst-case outage after a restart while keeping steady
state to one empty round trip per MCP per half minute.

### Nothing reconnects on the request path

The loop is the only thing that dials. A burst of calls arriving at a dead
downstream cannot become a burst of reconnect attempts, and the data-plane hot
path keeps its current shape — resolve, call, filter — with no dialing in it.

The cost is latency: worst-case recovery is one interval, during which calls
fail. That is the right trade for a gateway. An operator who wants faster
recovery lowers the interval, which is a bounded, predictable cost, rather than
accepting an unbounded one under load.

### Demotion keeps the cached schemas

A demoted registration holds `unreachable` with no client but keeps its tool
map. `Resolve` and `HasTool` both gate on `Status`, so a stale list cannot be
called — status is what makes an MCP uncallable, not an empty map. Dropping the
schemas would blank the admin UI's response-filter editor for the duration of an
outage, which is precisely the inert-endpoint symptom #180 was about.

### `503 mcp_unavailable` on the data plane

Demotion means `Resolve` now declines for MCPs that are registered and merely
down, and those were falling through to `404 mcp_not_found` — sending an operator
to look for a missing registration that was in fact fine. A registered MCP whose
downstream is unreachable now returns `503 mcp_unavailable`, which is also the
honest signal: the health loop is already trying to reconnect it, so the caller
should retry rather than conclude the MCP does not exist.

## Consequences

**Good:**

- An `http`/`sse` MCP survives a restart, redeploy or reschedule of the server
  behind it, unattended, within one interval.
- A boot-order race between the gateway and its downstreams stops mattering. The
  demo's TCP wait in `entrypoint.sh` becomes a convenience that keeps the first
  screenshot clean rather than a requirement.
- Registration status stops lying. An endpoint reading `active` can serve a call.
- A stdio MCP whose subprocess died is respawned instead of staying dead.
- The gap between "registered" and "reachable" is visible in both the admin API
  and the data plane's status code.

**Bad / accepted:**

- Up to one interval of failed calls after a downstream restart. Reconnecting on
  the request path would shrink that, at the cost of putting dialing under load
  in the hot path; see above.
- One `ping` per registered MCP per interval, forever. Empty round trips, but not
  free, and they show up in a downstream's access log.
- The stale-pooled-connection race is *absorbed* here, not fixed. A user's tool
  call can still pick the same dead connection out of the pool and get a
  `502 upstream_error`; only the health loop retries. Fixing that properly means
  either a retry policy on the call path or aligning `IdleConnTimeout` with what
  downstreams actually do, and belongs with ADR-0017's open "per-MCP
  timeout/retry configuration" item rather than here.
- A misconfigured endpoint is now retried rather than failing once. Backoff caps
  the rate (interval, doubling, capped at five minutes), but a stdio entry
  naming a command that does not exist will fork a doomed process on that
  schedule rather than never again.
- `cache.MCPClient` gains a `Ping` method, so every implementation (including
  test doubles) has to provide one.

## Alternatives considered

**Reconnect lazily, on a failed tool call.** Smallest change, and recovery is
immediate for the caller who triggers it. Rejected: a dead downstream is then
only ever discovered by a user request, status stays stale until someone calls,
and a burst of traffic against a restarted MCP turns into a burst of concurrent
dials on the request path.

**Extend `SchemaRefresher` to also reconnect.** One loop instead of two. Rejected:
it would have to be on by default to fix this, which reverses ADR-0013's
deliberate opt-in for schema refresh, and it conflates two concerns whose right
intervals differ by an order of magnitude.

**Let the sdk's transports reconnect themselves.** The Streamable HTTP transport
has resumption machinery for a dropped stream, but an MCP *session* is
server-side state: a server that restarted has never heard of the session ID the
client holds, and no amount of transport-level retry recovers it. The reconnect
has to redo `initialize`, which is a client-level concern.

**A `/healthz`-style probe per downstream.** Would require every MCP to expose
something outside the MCP protocol. `ping` is in the protocol and every server
answers it.

## References

- Issue #191
- [ADR-0003: Dynamic MCP registration and schema discovery](0003-dynamic-mcp-registration-and-schema-discovery.md)
- [ADR-0013: Periodic MCP schema refresh](0013-periodic-mcp-schema-refresh.md)
- [ADR-0017: HTTP and SSE transports for downstream MCPs](0017-http-and-sse-transports-for-downstream-mcps.md)
- `docs/features/mcp-health-check.md`
