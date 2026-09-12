# Control-Plane Admin API

The control-plane is the Gin-based HTTP surface described in
[ADR-0005](../architecture/decisions/0005-use-gin-for-control-plane-api.md). It
listens on `server.control_plane_addr` (see [`docs/CONFIG.md`](../CONFIG.md)) and is
a separate surface from the data plane — agents never talk to it. Bind it to a
trusted network/interface regardless of authentication.

## Authentication

Every endpoint below except `GET /admin/healthz` requires an
`Authorization: Bearer <jwt>` header when `admin_auth` is configured (see
[ADR-0010](../architecture/decisions/0010-control-plane-admin-authentication.md)
and [`CONFIG.md`](../CONFIG.md#admin-authentication)). The token is verified the
same way data-plane tokens are — signature against the `oidc:` JWKS, plus
`exp`/`iss`/`aud` — and its claims must then satisfy the `admin_auth.match`
rules.

| Status | `error` code            | When                                                        |
|--------|-------------------------|-------------------------------------------------------------|
| 401    | `missing_authorization` | No `Authorization` header.                                  |
| 401    | `invalid_authorization` | Header present but not a non-empty `Bearer` token.          |
| 401    | `unauthorized`          | Token fails signature / `exp` / `iss` / `aud` validation.   |
| 403    | `forbidden`             | Token valid but its claims don't satisfy `admin_auth.match`.|

These four responses are possible on **every** route documented below (again,
except `GET /admin/healthz` and `GET /admin/auth/config`) and are not repeated
in each endpoint's own status table. `GET /admin/swagger/*` is also behind this
gate.

The embedded web UI obtains its own token through an OIDC Authorization Code +
PKCE flow; see [`CONFIG.md`](../CONFIG.md#admin-ui-sign-in) and
[ADR-0014](../architecture/decisions/0014-admin-ui-oidc-pkce-login.md).

If `admin_auth` is not configured, the admin API is unauthenticated and the
gateway logs a startup warning — bind `control_plane_addr` to a trusted
interface. See [`CONFIG.md`](../CONFIG.md#admin-authentication).

Every write here goes through the live in-memory MCP Registry / Policy Engine first
(so the effect is immediate on the data plane), then persists via the GORM-backed
store (Epic #5) so it survives a restart.

Every error body has the shape `{"error": "<code>", "message": "<human-readable>"}`.

## `GET /admin/healthz`

Returns `200 OK` with body `ok` once the control-plane server has bound its
listener. No authentication required.

## `GET /admin/auth/config`

How a client should authenticate. **No authentication required** — a browser
that has no token yet has to be able to read it, which is the whole point;
gating it would be circular. It is one of exactly two open routes (the other
is `healthz`). See
[ADR-0014](../architecture/decisions/0014-admin-ui-oidc-pkce-login.md).

With `admin_auth` configured and `[admin_auth.login]` set:

```json
{
  "auth_required": true,
  "issuer": "https://auth.example.com",
  "client_id": "mcplake-admin-ui",
  "authorization_endpoint": "https://auth.example.com/authorize",
  "token_endpoint": "https://auth.example.com/oauth/token",
  "scopes": ["openid", "profile", "email"]
}
```

With admin auth off: `{"auth_required": false}`. With admin auth on but no
`[admin_auth.login]`: `{"auth_required": true, "issuer": "…"}` and no login
fields — a client must be able to tell "sign in like this" from "sign-in
isn't configured here".

Everything this endpoint returns is the **public** half of a PKCE client: the
`client_id` and the provider's endpoints travel in the authorization request
itself, visible to anyone who watches the redirect, and a public client has
no secret. It exposes no JWKS material, no policy content, and nothing about
the gateway's own state.

The embedded web UI calls this on load; a scripted client doesn't need it
(mint a token at your provider and send it as a bearer).

## Interactive API Reference

The full OpenAPI (Swagger 2.0) spec for this API is generated from the `@`
doc-comment annotations above each handler in `gateway/internal/controlplane`
via `swaggo/swag` (see
[ADR-0007](../architecture/decisions/0007-swaggo-for-control-plane-api-docs.md)),
and served interactively at `GET /admin/swagger/index.html` on the control-plane
port — same trust boundary as the rest of `/admin/*`. Regenerate it after
changing a handler's annotations with `make swagger` (requires the `swag` CLI:
`go install github.com/swaggo/swag/cmd/swag@latest`); `make swagger-check` fails
if the committed spec is stale.

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
| `enabled`          | bool   | no       | Operator on/off switch (default `true`). A disabled MCP still connects and caches its tools, but the data plane rejects every `POST /v1/call` for it with `403 mcp_disabled`. Toggle it later with `PATCH /admin/mcps/:name`. See [`CONFIG.md`](../CONFIG.md#4-mcp-servers). |

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
  "enabled": true,
  "tools": {
    "get_user": { "name": "get_user", "input_schema": {}, "output_schema": {} }
  }
}
```

`status` (`connecting`/`active`/`unreachable`) reflects connection health and is owned
by the gateway; `enabled` reflects operator intent and is only ever changed through
this API or the seed `config.toml`.

### `GET /admin/mcps`

Lists every currently registered MCP — reads the live Registry (in-memory), not the
database.

### `PATCH /admin/mcps/:name`

Enables or disables an already-registered MCP by flipping its `enabled` flag in the
live Registry (effective immediately on the data plane) and then persisting it. It
does **not** reconnect or re-discover the MCP: a disabled MCP keeps its cached tools
and live client, so re-enabling is instant. This is the reversible alternative to
`DELETE`, which drops the registration and its schema cache.

```
PATCH /admin/mcps/postgres-ro
Content-Type: application/json

{ "enabled": false }
```

| Field     | Type | Required | Description                     |
|-----------|------|----------|---------------------------------|
| `enabled` | bool | yes      | Desired state.                  |

| Status | Body `error` code | When                                              |
|--------|--------------------|--------------------------------------------------|
| 200    | —                  | Toggled; body is the updated registration (same shape as `POST`). |
| 400    | `invalid_request`  | Malformed JSON, or `enabled` missing.            |
| 404    | `not_found`        | No MCP is registered under that name.            |

While disabled, `POST /v1/call` for this MCP returns `403 mcp_disabled` (distinct from
`404 mcp_not_found`).

### `DELETE /admin/mcps/:name`

Unregisters an MCP: closes its client, removes it from the Registry, and deletes its
persisted row. In-flight calls to it are allowed to finish; new calls are rejected.

| Status | Body `error` code | When                                  |
|--------|--------------------|------------------------------------------|
| 204    | —                  | Unregistered.                            |
| 404    | `not_found`        | No MCP is registered under that name.    |

## Access Policies

Every write here persists via GORM and then refreshes the shared `router.PolicyStore`
engine (both access and filter policies together, since one engine covers both — see
[ADR-0004](../architecture/decisions/0004-unified-policy-engine-for-access-and-filtering.md)),
so the change is immediately visible to the data plane's `Authorize` calls.

### `POST /admin/access-policies`

```
POST /admin/access-policies
Content-Type: application/json

{
  "name": "db-reader",
  "match": [{ "path": "$.role", "pattern": "^db-reader$" }],
  "grants": [{ "mcp": "postgres-ro", "tools": ["*"] }]
}
```

`match` uses the same `{path, pattern}` JSONPath+regexp rule syntax as
`config.toml`'s `access_policies` (see [`docs/CONFIG.md`](../CONFIG.md#5-access-policies)
and [ADR-0002](../architecture/decisions/0002-jsonpath-regexp-claim-rule-engine.md)).

An optional `enabled` field (bool, default `true`) is accepted here and echoed in
every response. A disabled access policy is skipped during authorization — it grants
nothing, so a caller authorized only by it gets `403 forbidden`. See
[`CONFIG.md`](../CONFIG.md#5-access-policies).

| Status | Body `error` code | When                                                        |
|--------|--------------------|--------------------------------------------------------------|
| 201    | —                  | Created; body is the resulting policy.                       |
| 400    | `invalid_request`  | Malformed JSON, or `name` missing.                            |
| 400    | `invalid_policy`   | A `match` rule's `path`/`pattern` failed to compile — not persisted. |

### `GET /admin/access-policies`

Lists every stored access policy.

### `GET /admin/access-policies/:name`

Returns one policy, or `404 not_found`.

### `PUT /admin/access-policies/:name`

Replaces the named policy's `match`/`grants`/`enabled` in place (creates it if
absent). Same request body and `invalid_policy`/`invalid_request` responses as
`POST`. Because it is a full replace, omitting `enabled` resets it to `true` (the
policy is re-enabled).

### `DELETE /admin/access-policies/:name`

Removes the policy. `204` on success, `404 not_found` if no such policy exists.

## Filter Policies

Same shape and reload behavior as Access Policies above — a write here also
refreshes the shared `router.PolicyStore` engine (both policy kinds, together).

### `POST /admin/filter-policies`

```
POST /admin/filter-policies
Content-Type: application/json

{
  "name": "hide-pii-for-plain-users-get-user",
  "match": [{ "path": "$.role", "pattern": "^user$" }],
  "mcp": "postgres-ro",
  "tool": "get_user",
  "drop_fields": ["$.hashed_password", "$.api_key", "$.internal_id"]
}
```

`mcp`/`tool` are exact — no `"*"` wildcards, unlike access-policy grants (a filter
always targets one specific tool response shape). `drop_fields` are JSONPath
expressions into the tool's response body.

An optional `enabled` field (bool, default `true`) is accepted here and echoed in
every response. A disabled filter policy is skipped — it strips nothing, so a matching
response is returned unfiltered by it (other enabled policies for the same
`(mcp, tool)` still apply). See [`CONFIG.md`](../CONFIG.md#6-filter-policies).

| Status | Body `error` code | When                                                        |
|--------|--------------------|--------------------------------------------------------------|
| 201    | —                  | Created; body is the resulting policy.                       |
| 400    | `invalid_request`  | Malformed JSON, or `name`/`mcp`/`tool` missing.               |
| 400    | `invalid_policy`   | A `match` rule's `path`/`pattern` failed to compile — not persisted. |

### `GET /admin/filter-policies`

Lists every stored filter policy.

### `GET /admin/filter-policies/:name`

Returns one policy, or `404 not_found`.

### `PUT /admin/filter-policies/:name`

Replaces the named policy's `match`/`mcp`/`tool`/`drop_fields`/`enabled` in place
(creates it if absent). Same request body and error responses as `POST`. Because it
is a full replace, omitting `enabled` resets it to `true`.

### `DELETE /admin/filter-policies/:name`

Removes the policy. `204` on success, `404 not_found` if no such policy exists.
