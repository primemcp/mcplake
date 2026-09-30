# Enable / Disable Switch

Every runtime-managed object the gateway knows about — an **MCP registration**,
an **access policy**, a **filter policy** — carries an operator on/off flag,
`enabled` (default `true`). Turning one off is a reversible kill switch, not a
delete: the record stays, and for an MCP its discovered tool schemas and live
connection stay too, so re-enabling is instant.

Disabling means a different thing for each kind, because each sits at a
different point in the [request pipeline](../architecture/overview.rst#pipeline-per-request):

| Object disabled | Effect | Where |
|---|---|---|
| **MCP registration** | Every `POST /v1/call` for that MCP is rejected with `403 mcp_disabled` (distinct from `404 mcp_not_found`). No downstream call is made. The MCP stays connected and its tools stay cached. | data plane, after authorization, before routing |
| **Access policy** | The policy is skipped during authorization — it grants nothing. A caller authorized *only* by a disabled policy gets `403 forbidden` and no tool call happens: the pipeline never starts. Other enabled policies are unaffected. | `router.Engine.Authorize` |
| **Filter policy** | The policy is skipped — its `drop_fields` contribute nothing, so a response it would have stripped is returned with all fields intact. Other enabled filter policies for the same `(mcp, tool)` still apply. | `router.Engine.FieldsToRemove` |

For MCP registrations, `enabled` (operator intent) is separate from `status`
(`connecting` / `active` / `unreachable`, connection health owned by the
gateway). A disabled MCP can still be `active`; a disabled MCP that later goes
`unreachable` stays disabled — and the
[health check](mcp-health-check.md) reconnects it like any other, so
re-enabling it still needs no reconnect. The health loop never touches
`enabled`.

`enabled` is *only* ever changed through configuration or the admin API — it is
never inferred. An existing deployment upgrading to a build with this feature
changes no behaviour until an operator flips a flag: an omitted config key, a
pre-existing database row, and an admin request without the field all mean
"enabled".

## Setting it

**Seed config** (`config.toml`) — an optional `enabled` key on any entry in the
`[[mcps]]`, `[[access_policies]]`, or `[[filter_policies]]` arrays. See
[CONFIG.md](../reference/configuration.rst#4-mcp-servers) and `config.example.toml`.

```toml
[[mcps]]
name = "filesystem"
command = "mcp-server-filesystem"
enabled = false
```

**Admin API** ([admin.md](../reference/admin-api.rst)) — the database is the source of
truth once the gateway is running, and stays so across restarts: config is
seeded once per entry and never re-applied over a runtime change
([ADR-0016](../architecture/decisions/0016-config-seeding-happens-once-per-entry.rst)):

- Policies: `enabled` is a field on `POST` / `PUT /admin/access-policies` and
  `/admin/filter-policies`, and is echoed in every response. `PUT` is a full
  replace, so omitting `enabled` re-enables the policy.
- MCPs: `enabled` on `POST /admin/mcps`, and a dedicated
  `PATCH /admin/mcps/:name` with body `{"enabled": <bool>}` that flips the
  flag in the live registry (effective immediately on the data plane) and
  persists it, without reconnecting.

Every write takes effect on the running data plane immediately — the same
live-refresh path every other admin write uses.

## Relationship to delete

Disabling an MCP is the reversible alternative to `DELETE /admin/mcps/:name`,
which removes the registration *and* its cached tool schemas and forces a full
re-register (reconnect + re-discover) to bring it back. Use `enabled = false` to
pause an MCP you intend to bring back; use `DELETE` to retire one for good.
