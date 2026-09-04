# Control-Plane Admin API

The control-plane is the Gin-based HTTP surface described in
[ADR-0005](../architecture/decisions/0005-use-gin-for-control-plane-api.md). It
listens on `server.control_plane_addr` (see [`docs/CONFIG.md`](../CONFIG.md)) and is
a separate surface from the data plane — agents never talk to it. Bind it to a
trusted network/interface only: there is no admin authN/authZ yet (tracked as an
ADR-0005 follow-up).

Every write here goes through the live in-memory MCP Registry / Policy Engine first
(so the effect is immediate on the data plane), then persists via the GORM-backed
store (Epic #5) so it survives a restart.

Every error body has the shape `{"error": "<code>", "message": "<human-readable>"}`.

## `GET /admin/healthz`

Returns `200 OK` with body `ok` once the control-plane server has bound its
listener. No authentication required.

## MCP Registrations

### `POST /admin/mcps`

Registers a downstream MCP: connects, discovers its tools (`tools/list`), and makes
it immediately callable via the data plane. See
[ADR-0003](../architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery.md).

```
POST /admin/mcps
Content-Type: application/json

{
  "name": "postgres-ro",
  "transport": "stdio",
  "connect": {
    "command": "mcp-server-postgres",
    "arguments": ["--connection-string", "postgresql://reader@localhost/mydb", "--read-only"]
  }
}
```

| Field              | Type   | Required | Description                                    |
|--------------------|--------|----------|--------------------------------------------------|
| `name`             | string | yes      | Unique MCP identifier.                            |
| `transport`        | string | no       | `stdio` (default), `sse`, or `http`.              |
| `connect.command`  | string | for stdio| Binary to execute.                                |
| `connect.arguments`| array  | no       | Command-line arguments.                           |
| `connect.url`      | string | for sse/http | Endpoint URL.                                |

| Status | Body `error` code    | When                                                    |
|--------|-----------------------|-----------------------------------------------------------|
| 201    | —                     | Registered; body is the resulting registration (below).  |
| 400    | `invalid_request`     | Malformed JSON, or `name` missing.                        |
| 502    | `registration_failed` | Connecting to or discovering tools on the MCP failed — nothing is persisted or made visible to the data plane. |

Response body (also the shape returned by `GET /admin/mcps`):

```json
{
  "name": "postgres-ro",
  "transport": "stdio",
  "connect": { "command": "mcp-server-postgres", "arguments": ["--read-only"] },
  "status": "active",
  "tools": {
    "get_user": { "name": "get_user", "input_schema": {}, "output_schema": {} }
  }
}
```

### `GET /admin/mcps`

Lists every currently registered MCP — reads the live Registry (in-memory), not the
database.

### `DELETE /admin/mcps/:name`

Unregisters an MCP: closes its client, removes it from the Registry, and deletes its
persisted row. In-flight calls to it are allowed to finish; new calls are rejected.

| Status | Body `error` code | When                                  |
|--------|--------------------|------------------------------------------|
| 204    | —                  | Unregistered.                            |
| 404    | `not_found`        | No MCP is registered under that name.    |
