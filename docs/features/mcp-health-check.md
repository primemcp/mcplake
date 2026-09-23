# MCP Health Check and Reconnection

The gateway builds a session to each downstream MCP once, when the MCP is
registered ([ADR-0003](../architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery.md)).
A server reached over `http` or `sse` ([ADR-0017](../architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps.md))
has a lifecycle the gateway neither owns nor is told about: it restarts, gets
redeployed, gets rescheduled. When that happens the session the gateway is
holding is dead, and nothing about it says so.

The health check is what notices. Every interval it pings each registered MCP
and rebuilds the ones that have stopped answering — and retries the ones that
were never reachable in the first place.

See [ADR-0019](../architecture/decisions/0019-reconnect-downstream-mcps-on-a-health-check-loop.md)
for the design and the alternatives considered.

## Configuring it

The loop is **on by default**. It is configured in the `[mcp]` table of
`config.toml`:

```toml
[mcp]
health_check_interval = "30s"
```

- The value is a Go duration string (`"10s"`, `"30s"`, `"2m"`).
- **Omitted means `"30s"`** — unlike
  [`schema_refresh_interval`](mcp-schema-refresh.md), which is off unless you
  ask for it.
- **`"0"` disables it.** The gateway logs a `WARN` at startup saying so, because
  with the loop off a downstream MCP that restarts stays broken until the
  gateway is restarted.
- A negative value is rejected at startup.

Changing the interval requires a restart; it is read once, when the app is
built.

## What a tick does

For each registered MCP, whatever its transport:

| State found | What happens |
| --- | --- |
| `active`, session answers `ping` | Nothing. One empty round trip. |
| `active`, session answers `ping` with `-32601 Method not found` | Nothing. The server does not implement `ping`, but it *answered on this session*, which is exactly what the probe is asking. |
| `active`, session does not answer | The ping is retried once first — a single failure is usually a pooled TCP connection the server closed rather than a dead session. If the retry also fails, reconnect. On success the MCP **stays** `active` — the client and tool schemas are swapped underneath it, so a concurrent caller sees the old working client or the new one, never a gap. The superseded client is closed. |
| not `active` (`unreachable`) | Reconnect. On success it becomes `active` with a freshly discovered tool list. This is how an MCP registered before its server existed heals itself. |
| reconnect fails | The MCP is demoted to `unreachable`, its dead client is closed, and it is retried on a backoff. |

Reconnecting re-runs the full registration handshake — connect, `initialize`,
`tools/list` — against whatever the registration currently says, so a URL or
command edited through the admin API is what gets dialed.

`enabled` is operator intent and is never changed by a health check: a disabled
MCP that reconnects stays disabled.

### Backoff

An MCP that cannot be reconnected is retried after the interval, then double
that, doubling on each consecutive failure up to a cap of **five minutes**. A
successful check clears the penalty immediately. So a downstream that is down
for an afternoon is dialed a few dozen times rather than a few thousand, and
still comes back within five minutes of returning.

Each failed attempt logs one `WARN` naming the MCP, the consecutive-failure
count, the next retry delay and the underlying error.

## What an operator sees

A registration's `status` now tracks reachability rather than only reflecting
how registration went:

- `active` — there is a live session. A call can be served.
- `unreachable` — registered, no live session, being retried.

`GET /admin/mcps` and the admin UI show this directly. An `unreachable`
registration **keeps its cached tool schemas**, so the response-filter editor
still works on an endpoint that is merely down; `status` is what makes it
uncallable, not an empty tool list.

On the data plane, a call to an MCP in that state returns:

```json
{"error": "mcp_unavailable", "message": "mcp is registered but not currently reachable"}
```

with `503 Service Unavailable`. That is deliberately distinct from
`404 mcp_not_found`, which means no such registration exists. A `503` is worth
retrying; the health loop is already working on it.

## Choosing an interval

- The cost is one `ping` per registered MCP per interval — an empty round trip,
  but it does appear in a downstream's access log.
- The worst-case outage after a downstream restart is one interval. Nothing
  reconnects on the request path, by design, so lowering the interval is the
  only way to shorten it.
- Seconds are reasonable here, unlike `schema_refresh_interval`. A `ping` is far
  cheaper than a `tools/list`.

## Servers that do not implement `ping`

`ping` is in the MCP spec and the official SDKs answer it, but plenty of
deployed servers do not, replying `-32601 Method not found`. That counts as
healthy here, and deliberately so: the request reached the server, was decoded,
was matched against its method table and was answered on this session — which is
the whole question a liveness probe asks.

You do not need to configure anything for such a server, and it is not
second-class: it gets the same detection as any other, because once it really
goes away the failure is at the transport, not a `-32601`.

Any *other* JSON-RPC error to a ping is treated as a dead session, because a
server answering, say, `-32603` to an empty request is not obviously well.

## Failure modes this does *not* cover

- **A downstream that accepts connections and answers `ping` but fails real
  calls.** Health is liveness, not correctness.
- **A tool set that changed while the session stayed alive.** That is
  [periodic schema refresh](mcp-schema-refresh.md)'s job, on its own interval.
  A *reconnect* does re-discover tools, but only because it has to build the
  registration again.
- **A misconfigured registration.** It will be retried forever on the capped
  backoff rather than failing once. Fix the registration or delete it.
- **A stale pooled connection hit by a *user's* call.** The health check retries
  its own probe, but a tool call that picks a connection the downstream already
  closed still fails with `502 upstream_error`; retry it. This is tracked as
  follow-up on [ADR-0017](../architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps.md).

## Related

- [CONFIG.md](../CONFIG.md#mcp-global) — the `[mcp]` table.
- [data-plane.md](../api/data-plane.md) — `503 mcp_unavailable`.
- [mcp-schema-refresh.md](mcp-schema-refresh.md) — the other `[mcp]` timer, and
  why they are separate.
- [enable-disable.md](enable-disable.md) — `enabled` vs. `status`.
- [ADR-0019](../architecture/decisions/0019-reconnect-downstream-mcps-on-a-health-check-loop.md).
