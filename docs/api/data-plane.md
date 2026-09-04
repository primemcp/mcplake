# Data-Plane API

The data-plane is the fasthttp server described in
[ADR-0001](../architecture/decisions/0001-use-fasthttp-for-gateway-server.md).
It listens on `server.data_plane_addr` (see [`docs/CONFIG.md`](../CONFIG.md)) and
is the only surface agents talk to — never the control-plane admin API, never a
downstream MCP directly.

## `GET /healthz`

Returns `200 OK` with body `ok` once the gateway has bound its listener. No
authentication required.

## `POST /v1/call`

Invokes a tool on a registered downstream MCP, subject to the caller's JWT-derived
access policy (see [ADR-0004](../architecture/decisions/0004-unified-policy-engine-for-access-and-filtering.md)).

### Request

```
POST /v1/call
Authorization: Bearer <jwt>
Content-Type: application/json

{
  "mcp": "postgres-ro",
  "tool": "get_user",
  "arguments": { "id": 42 }
}
```

| Field       | Type   | Required | Description                                  |
|-------------|--------|----------|-----------------------------------------------|
| `mcp`       | string | yes      | Registered MCP name to route the call to.     |
| `tool`      | string | yes      | Tool name to invoke on that MCP.               |
| `arguments` | object | no       | Passed through to the MCP tool call.           |

The `Authorization` header is required and must use the `Bearer` scheme.

The full pipeline (auth → authorize → route → call → filter) is described in
[data.md#request-lifecycle](../architecture/data.md#request-lifecycle).

### Responses

| Status | Body `error` code       | When                                                          |
|--------|--------------------------|----------------------------------------------------------------|
| 200    | —                        | Tool call succeeded; body is the (filtered) tool response.     |
| 400    | `invalid_json`           | Request body is not valid JSON.                                 |
| 400    | `missing_field`          | `mcp` or `tool` is missing/empty.                               |
| 401    | `missing_authorization`  | `Authorization` header is absent.                                |
| 401    | `invalid_authorization`  | Header present but not a (non-empty) `Bearer` token.             |
| 401    | `unauthorized`           | Token present but fails signature/exp/iss/aud validation.        |
| 403    | `forbidden`              | Token valid but claims don't authorize this `(mcp, tool)` (no `AccessPolicy` grants it). |
| 404    | `mcp_not_found`          | `mcp` isn't registered, or isn't currently active.               |
| 404    | `tool_not_found`         | `mcp` exists but doesn't advertise `tool`.                       |
| 502    | `upstream_error`         | The downstream MCP call itself failed (connection issue, tool-level error). |
| 504    | `upstream_timeout`       | The downstream MCP call didn't complete within the gateway's call timeout (`internal.Config.CallTimeout`, default 30s — not yet exposed as a `config.yaml` key). |
| 500    | `internal_error`         | The Policy Engine or response filter failed unexpectedly — not a caller error. |
| 501    | `not_implemented`        | The gateway was started without a configured auth/policy/MCP pipeline (should not happen in a real deployment). |

Every error body has the shape `{"error": "<code>", "message": "<human-readable>"}`.

Authorization is checked *before* existence: a caller lacking a grant for a given
`(mcp, tool)` gets 403 even if that `mcp`/`tool` doesn't actually exist, matching the
`JWT eval -> [mcps] -> [tools]` pipeline order in
[overview.md](../architecture/overview.md#pipeline-per-request) — access policies are
evaluated purely against claims and the requested names, independent of whether the
registry currently has anything registered under them.

### Current limitations (tracked, not bugs)

- No request size limit or rate limiting is documented yet.
