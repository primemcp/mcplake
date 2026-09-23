# Periodic MCP Schema Refresh

The gateway discovers a downstream MCP's tools once, when the MCP is
registered ([ADR-0003](../architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery.md)).
On a long-running gateway that view can drift: a tool added to an MCP stays
invisible, and a tool removed from an MCP still looks callable (the call then
fails at the MCP). Periodic schema refresh closes that gap on a timer.

See [ADR-0013](../architecture/decisions/0013-periodic-mcp-schema-refresh.md)
for the design and the alternatives considered.

## Enabling it

Set an interval in the `[mcp]` table of `config.toml`:

```toml
[mcp]
schema_refresh_interval = "15m"
```

- The value is a Go duration string (`"30s"`, `"5m"`, `"1h"`).
- **Omitted or `"0"` disables it** — this is the default, and matches the
  behaviour before this feature: schemas are fetched only at registration.
- A negative value is rejected at startup.

## What a refresh does

Every interval, for each MCP that is currently `active`:

1. The gateway calls `tools/list` again on the **existing** client — the same
   connection (and, for a stdio MCP, the same subprocess). Nothing reconnects.
2. On success, the MCP's cached tool set is atomically replaced with the fresh
   one. A newly-advertised tool becomes routable; a removed tool starts
   returning `404 tool_not_found` instead of a downstream `502`.
3. On failure (the MCP is briefly busy or unreachable), the error is logged at
   `WARN` and the **previous** schema and `status` are kept untouched — a
   transient blip never takes a working MCP out of rotation. The next tick
   retries.

Operator intent (`enabled`) and connection health (`status`) are never changed
by a refresh. A disabled MCP is still `active` (it stays connected — see
[enable/disable](enable-disable.md)), so its schema is kept current for the
moment it is re-enabled.

## Choosing an interval

- The cost is one `tools/list` round-trip per active MCP per interval. Pick
  minutes, not seconds.
- The staleness window is up to one interval — this is polling, not push. A
  tool that appears and disappears within a single interval is never observed.
- For an immediate refresh after a known downstream change, re-register the MCP
  (`DELETE` then `POST /admin/mcps`); an on-demand refresh endpoint is tracked
  as follow-up in ADR-0013.

## What it does *not* do

A refresh never reconnects. It calls `tools/list` on the client the
registration already holds, and if that client's session is dead the refresh
fails and is logged — the previous schema stays put. Detecting and repairing a
dead session is
[the health check](mcp-health-check.md)'s job, on its own interval.

## Related

- [mcp-health-check.md](mcp-health-check.md) — the other `[mcp]` timer. Schema
  freshness and connection health are separate questions; note that the health
  check is **on by default** and this one is not.
- [CONFIG.md](../CONFIG.md#mcp-global) — the `[mcp]` table.
- [enable-disable.md](enable-disable.md) — why a disabled MCP is still refreshed.
- [ADR-0013](../architecture/decisions/0013-periodic-mcp-schema-refresh.md).
