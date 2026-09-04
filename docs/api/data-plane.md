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

### Responses

| Status | Body `error` code       | When                                                        |
|--------|--------------------------|--------------------------------------------------------------|
| 200    | —                        | Tool call succeeded; body is the (filtered) tool response.   |
| 400    | `invalid_json`           | Request body is not valid JSON.                               |
| 400    | `missing_field`          | `mcp` or `tool` is missing/empty.                             |
| 401    | `missing_authorization`  | `Authorization` header is absent.                              |
| 401    | `invalid_authorization`  | Header present but not a (non-empty) `Bearer` token.           |
| 401    | *(auth.ErrUnauthorized)* | Token present but fails signature/exp/iss/aud validation.      |
| 403    | *(pending #9)*           | Token valid but claims don't authorize this `(mcp, tool)`.     |
| 404    | *(pending #9)*           | `mcp` or `tool` doesn't exist.                                 |
| 501    | `not_implemented`        | Request is well-formed; the pipeline behind it isn't wired yet (temporary, removed by #9). |

Every error body has the shape `{"error": "<code>", "message": "<human-readable>"}`.
The `501` stub additionally echoes `mcp`/`tool` back, purely to help manually
verify request parsing before the real pipeline exists — this field is not part of
the stable contract and will disappear once #9 lands.

### Current limitations (tracked, not bugs)

- The 401/403/404 branches for signature/claims/routing failures are not wired yet
  — see [Epic #1](https://github.com/atsokha/mcplake/issues/1), tickets #9-#11.
- No request size limit or rate limiting is documented yet.
