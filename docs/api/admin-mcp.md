# MCP Control Server

The MCP control server exposes every [control-plane admin API](admin.md)
operation as an **MCP tool**, so an MCP-speaking client (an agent, an
MCP-enabled editor) can administer a running gateway the same way it calls any
other tool. It is a thin adapter over the same application layer the REST
handlers use, so the two surfaces always behave identically. See
[ADR-0011](../architecture/decisions/0011-mcp-control-server.md).

It is **off by default**. Enable it in `config.yaml`:

```yaml
admin_mcp:
  enabled: true
  path: /admin/mcp   # optional, default /admin/mcp; must be under /admin/
```

## Connecting

- **Transport:** streamable HTTP, mounted on the **control-plane** listener
  (`server.control_plane_addr`) at `admin_mcp.path`.
- **Auth:** the endpoint is under `/admin/`, so it is gated by
  [`admin_auth`](admin.md#authentication) exactly like the rest of the admin
  API. The MCP client must send `Authorization: Bearer <jwt>` with the initial
  request; a token that fails validation or the `admin_auth.match` rules is
  rejected (`401`/`403`) before the MCP handshake starts. If `admin_auth` is not
  configured, the control server is unauthenticated (the gateway logs a warning
  at startup) — bind the control-plane listener to a trusted interface.
- Example (Go, `github.com/modelcontextprotocol/go-sdk`):

  ```go
  client := mcp.NewClient(&mcp.Implementation{Name: "ops", Version: "1"}, nil)
  tr := &mcp.StreamableClientTransport{
      Endpoint:   "http://gateway.internal:8081/admin/mcp",
      HTTPClient: bearerClient(adminJWT), // an *http.Client that sets the header
  }
  session, err := client.Connect(ctx, tr, nil)
  ```

## Behaviour

- Every tool maps 1:1 to a REST endpoint and goes through the identical
  write-through path: the live in-memory Registry / Policy Engine is updated
  first (so the data plane sees the change immediately), then the durable store,
  then a policy-engine refresh for policy changes.
- A failure the caller can act on — unknown name, a claim rule that won't
  compile, a downstream MCP that won't connect — is returned as an **MCP tool
  error** (`isError: true`) with a human-readable message, not a protocol
  error, so an LLM caller can see it and self-correct.
- Inputs and outputs are structured; the schemas are published via `tools/list`.

## Tool catalog

### MCP registrations

| Tool | REST equivalent | Input | Output |
|------|-----------------|-------|--------|
| `list_mcps` | `GET /admin/mcps` | — | `{ mcps: [MCP] }` |
| `register_mcp` | `POST /admin/mcps` | `{ name, transport?, command?, arguments?, url?, enabled? }` | `MCP` |
| `set_mcp_enabled` | `PATCH /admin/mcps/:name` | `{ name, enabled }` | `MCP` |
| `unregister_mcp` | `DELETE /admin/mcps/:name` | `{ name }` | `{ ok }` |

`MCP` = `{ name, transport, command?, arguments?, url?, status, enabled, tools: [{ name, input_schema?, output_schema? }] }`.

- `register_mcp` connects to the MCP, discovers its tools, and makes it callable
  through the data plane; on a connect/discover failure nothing is persisted and
  the tool returns an error. `transport` defaults to `stdio`.
- `set_mcp_enabled` toggles the operator flag in place without reconnecting; a
  disabled MCP keeps its cached tools and the data plane rejects calls to it with
  `403 mcp_disabled`.
- `unregister_mcp` drops the registration and its schema cache — use
  `set_mcp_enabled` to pause reversibly.

### Access policies

| Tool | REST equivalent | Input | Output |
|------|-----------------|-------|--------|
| `list_access_policies` | `GET /admin/access-policies` | — | `{ policies: [AccessPolicy] }` |
| `get_access_policy` | `GET /admin/access-policies/:name` | `{ name }` | `AccessPolicy` |
| `create_access_policy` | `POST /admin/access-policies` | `AccessPolicyInput` | `AccessPolicy` |
| `replace_access_policy` | `PUT /admin/access-policies/:name` | `AccessPolicyInput` | `AccessPolicy` |
| `delete_access_policy` | `DELETE /admin/access-policies/:name` | `{ name }` | `{ ok }` |

`AccessPolicyInput` = `{ name, match: [{ path, pattern }], grants: [{ mcp, tools }], enabled? }`.
`create_*` and `replace_*` are both upserts (add-or-overwrite by name); `create`
mirrors the REST `POST`, `replace` the REST `PUT`. A `match` rule whose `path` or
`pattern` fails to compile is rejected and nothing is stored.

### Filter policies

| Tool | REST equivalent | Input | Output |
|------|-----------------|-------|--------|
| `list_filter_policies` | `GET /admin/filter-policies` | — | `{ policies: [FilterPolicy] }` |
| `get_filter_policy` | `GET /admin/filter-policies/:name` | `{ name }` | `FilterPolicy` |
| `create_filter_policy` | `POST /admin/filter-policies` | `FilterPolicyInput` | `FilterPolicy` |
| `replace_filter_policy` | `PUT /admin/filter-policies/:name` | `FilterPolicyInput` | `FilterPolicy` |
| `delete_filter_policy` | `DELETE /admin/filter-policies/:name` | `{ name }` | `{ ok }` |

`FilterPolicyInput` = `{ name, match: [{ path, pattern }], mcp, tool, drop_fields: [jsonpath], enabled? }`.
`mcp` and `tool` are exact — no wildcards.

### Health

| Tool | REST equivalent | Input | Output |
|------|-----------------|-------|--------|
| `gateway_health` | `GET /admin/healthz` | — | `{ status: "ok" }` |

## Example: register an MCP, then list

```jsonc
// tools/call register_mcp
{ "name": "postgres-ro",
  "command": "mcp-server-postgres",
  "arguments": ["--connection-string", "postgresql://reader@localhost/db", "--read-only"] }
// -> { "name": "postgres-ro", "transport": "stdio", "status": "active",
//      "enabled": true, "tools": [{ "name": "get_user", ... }] }

// tools/call list_mcps
{}
// -> { "mcps": [ { "name": "postgres-ro", "status": "active", "enabled": true, ... } ] }
```

The registration is now live on the data plane — a `POST /v1/call` for
`postgres-ro` is routable immediately, subject to the caller's access policy.

## See also

- [Control-Plane Admin API](admin.md) — the REST surface these tools mirror.
- [CONFIG.md](../CONFIG.md#admin-mcp) — the `admin_mcp` config keys.
- [ADR-0011](../architecture/decisions/0011-mcp-control-server.md) — the design.
