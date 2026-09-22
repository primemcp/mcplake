# MCP Gateway Configuration Guide

## Overview

The MCP Gateway is configured with a single **TOML** file
([ADR-0012](architecture/decisions/0012-toml-configuration-format.md)), passed
with `--config` (default `config.toml`). This guide covers every option. See
[`config.example.toml`](../config.example.toml) for a complete file.

## Configuration Sections

### 1. Server Configuration

Controls the gateway's two HTTP surfaces — see
[`docs/architecture/overview.md`](architecture/overview.md) for why they're
separate.

```toml
[server]
data_plane_addr = ":8080"     # fasthttp tool-call proxy (ADR-0001)
control_plane_addr = ":8081"  # Gin admin API (ADR-0005)
# tls.cert_file = "/path/to/cert.pem"
# tls.key_file  = "/path/to/key.pem"
```

**Fields:**
- `data_plane_addr` — listen address for the tool-call endpoint used by agents
  (`POST /v1/call`, `GET /healthz`). Required.
- `control_plane_addr` — listen address for the admin API used to manage MCP
  registrations and policies (`/admin/*`). Bind this to a trusted network/interface
  only — see [ADR-0005](architecture/decisions/0005-use-gin-for-control-plane-api.md).

### 2. OIDC Configuration

Configures JWT validation and OIDC provider integration.

```toml
[oidc]
jwks_url = "https://auth.example.com/.well-known/jwks.json"
issuer = "https://auth.example.com"
audience = "mcp-gateway"
# jwks_cache_ttl = "1h"
```

**Fields:**
- `jwks_url` — the OIDC provider's JWKS endpoint (required). The gateway
  currently requires this exact URL; it does not yet perform OIDC discovery
  from a provider/issuer URL (`/.well-known/openid-configuration`) — that's
  tracked as future work, not implemented in `auth.Validator` yet.
- `issuer` — required `iss` claim value (required)
- `audience` — required `aud` claim value (required)
- `jwks_cache_ttl` — a Go duration string (`"1h"`, `"30m"`) for how long
  fetched keys are cached before a background refresh (default: `1h` when
  omitted or `"0"`). The cache is also refreshed out-of-band, rate-limited
  to once per minute, whenever a token references an unrecognized key ID
  (e.g. right after the provider rotates its signing key) — see
  [`auth.Validator`](../auth/validator.go).

Every JWT validation failure (bad signature, expired, wrong `iss`/`aud`,
malformed token) is reported the same way — as `auth.ErrUnauthorized` — and
maps to a 401 at the gateway's tool-call endpoint.

### Admin Authentication

Authenticates and authorizes callers of the control-plane admin API
(`/admin/*`). See
[ADR-0010](architecture/decisions/0010-control-plane-admin-authentication.md).

```toml
# [admin_auth]
# enabled = true          # optional; see the default rule below

[[admin_auth.match]]
path = "$.role"           # JSONPath into the decoded claim set
pattern = "^admin$"       # regexp tested against the extracted value(s)

[[admin_auth.match]]
path = "$.iss"
pattern = "^https://auth\\.example\\.com$"
```

**How it works:**
1. Every request to `/admin/*` except `GET /admin/healthz` must carry an
   `Authorization: Bearer <jwt>` header.
2. The token is verified exactly like a data-plane token — signature against
   the `[oidc]` JWKS, plus `exp`/`iss`/`aud`. There is no separate admin
   provider; the admin surface reuses the `[oidc]` section above.
3. The decoded claim set is then tested against `admin_auth.match` — the same
   `{path, pattern}` JSONPath+regexp rule syntax as `access_policies`
   ([ADR-0002](architecture/decisions/0002-jsonpath-regexp-claim-rule-engine.md)).
   All rules are ANDed. A caller whose claims fail any rule gets `403`.

**Fields:**
- `match` — a list of `{path, pattern}` rules (each `[[admin_auth.match]]`
  block is one rule), ANDed. Pin a claim that only admin tokens carry (a role,
  a group, a dedicated `aud`); an over-broad rule makes every authenticated
  user an admin. Anchor every pattern with `^…$`.
- `enabled` — optional on/off switch.
  - **Omitted:** admin auth is **on** iff `match` has at least one rule. So a
    config with no `admin_auth` section at all leaves `/admin/*` **open**, and
    the gateway logs a prominent `WARN` at startup. This is the
    backward-compatible default; bind `control_plane_addr` to a trusted
    interface until you configure `match`.
  - `enabled = false` — force admin auth off even if `match` rules are present.
  - `enabled = true` with an empty `match` is a configuration error (it would
    authorize every authenticated caller) and fails at startup.

**Consequences:**
- `GET /admin/swagger/*` (the interactive API explorer) is behind this gate
  once admin auth is on.
- The embedded admin web UI signs operators in through the OIDC provider and
  attaches the resulting token — configure `[admin_auth.login]` below, or the
  UI will have no way to obtain one.
- A malformed `path`/`pattern` fails `Config.Validate()` at startup, like
  every other claim rule.

#### Admin UI sign-in

How the embedded [admin web UI](features/admin-webui.md) obtains an admin
token: an OIDC **Authorization Code + PKCE** flow, run in the browser against
the same provider `[oidc]` already verifies tokens from. See
[ADR-0014](architecture/decisions/0014-admin-ui-oidc-pkce-login.md).

```toml
[admin_auth.login]
client_id = "mcplake-admin-ui"
authorization_endpoint = "https://auth.example.com/authorize"
token_endpoint = "https://auth.example.com/oauth/token"
# scopes = ["openid", "profile", "email"]   # optional; this is the default
```

**Fields:**
- `client_id` — a **public** client registered at your provider for this UI
  (no client secret: the UI's bundle is readable by anyone who loads the
  page, which is the case PKCE exists for). Its allowed redirect URIs must
  include the control-plane URL operators browse to, with a trailing slash —
  e.g. `https://gateway.internal:8081/`.
- `authorization_endpoint` / `token_endpoint` — absolute `http(s)` URLs,
  taken from your provider's own discovery document. They are configured
  explicitly rather than discovered, for the same reason `oidc.jwks_url` is:
  the gateway makes no outbound calls of its own at startup.
- `scopes` — optional; defaults to `["openid", "profile", "email"]`. The
  token the provider issues must satisfy `oidc.audience`, which for many
  providers is a matter of scope or a provider-side audience mapping.

**Behaviour:**
- Omit the whole section and the UI reports that sign-in isn't configured;
  the gateway logs a `WARN` at startup saying the same. A **partially**
  filled section is a startup error, not a silent "off".
- With admin auth off (no `admin_auth.match`), this section is ignored and
  the UI shows no sign-in at all.
- The UI serves `GET /admin/auth/config` — unauthenticated — to discover
  these values; see [api/admin.md](api/admin.md#get-adminauthconfig). It
  exposes only the public half of a PKCE client.

### Admin MCP

Enables the in-process **MCP control server** — every admin operation exposed as
an MCP tool on the control-plane listener, for MCP-speaking clients. See
[ADR-0011](architecture/decisions/0011-mcp-control-server.md) and
[`docs/api/admin-mcp.md`](api/admin-mcp.md).

```toml
[admin_mcp]
enabled = true
# path = "/admin/mcp"
```

**Fields:**
- `enabled` — off by default; an omitted `admin_mcp` section mounts nothing.
- `path` — where the streamable-HTTP handler mounts. Default `/admin/mcp`. Must
  start with `/admin/` so the `admin_auth` gate covers it;
  `Config.Validate()` rejects anything else at startup.

The MCP endpoint is under `/admin/`, so it is authenticated and claim-gated by
`admin_auth` exactly like the REST admin API. With `admin_mcp.enabled = true` and
no `admin_auth`, the control server is unauthenticated and the gateway logs a
warning — bind `control_plane_addr` to a trusted interface.

### MCP (global)

Settings that apply to every registered MCP.

```toml
[mcp]
schema_refresh_interval = "15m"
```

**Fields:**
- `schema_refresh_interval` — a Go duration string. When set, the gateway
  re-runs `tools/list` on each active MCP's **existing** client on this
  interval (no reconnect) and swaps in the fresh schema, so a tool added or
  removed downstream is picked up within roughly one interval. Omitted or
  `"0"` disables periodic refresh (the default — schemas are then fetched only
  at registration). A `tools/list` failure during a refresh is logged and
  leaves the previous schema in place. See
  [ADR-0013](architecture/decisions/0013-periodic-mcp-schema-refresh.md).

### 3. Persistence

Configures the control-plane's durable store for MCP registrations and
access/filter policies. See
[ADR-0006](architecture/decisions/0006-gorm-sqlite-postgres-persistence.md).

```toml
[persistence]
driver = "sqlite"    # sqlite (default) | postgres
dsn = "gateway.db"   # SQLite file path, or a Postgres connection string
```

**Fields:**
- `driver` — `sqlite` (default) or `postgres`. SQLite runs embedded via a
  pure-Go driver (no CGO, no external service) — the default for a
  single-instance, air-gapped deployment. `postgres` is for distributed
  deployments sharing one control-plane store.
- `dsn` — the SQLite file path when `driver = "sqlite"`, or a PostgreSQL
  connection string when `driver = "postgres"`.

At startup, this section's entries plus the `[[mcps]]` / `[[access_policies]]` /
`[[filter_policies]]` arrays below are upserted into this store by name — the
database becomes the source of truth from then on; the config file is a seed
mechanism, not a parallel state store.

### 4. MCP Servers

Defines all MCP servers the gateway connects to. Each server is one
`[[mcps]]` block.

```toml
[[mcps]]
name = "postgres-ro"
type = "stdio"               # stdio, sse, etc.
command = "mcp-server-postgres"
arguments = ["--connection-string", "postgresql://user@localhost/db", "--read-only"]

[[mcps]]
name = "filesystem"
type = "stdio"
command = "mcp-server-filesystem"
arguments = ["/data"]
```

**Fields:**
- `name` — Unique identifier for this MCP instance (required)
- `type` — Transport type (default: `"stdio"`)
- `command` — Binary to execute (required)
- `arguments` — Command-line arguments
- `enabled` — operator on/off switch (optional, default `true`). When
  `false`, the MCP is still registered and (at startup) still connected, its
  tools stay cached, but the data plane rejects every `POST /v1/call` for it
  with `403 mcp_disabled` — all pipelines stop for this MCP. Re-enabling
  needs no reconnect. This is distinct from omitting the entry or calling
  `DELETE /admin/mcps/:name`, which drop the registration and its schema
  cache entirely. `enabled` reflects operator intent; the separate `status`
  field (`connecting`/`active`/`unreachable`) reflects connection health.

### 5. Access Policies

Grants access to `(mcp, tool)` pairs based on JWT claims. See
[ADR-0002](architecture/decisions/0002-jsonpath-regexp-claim-rule-engine.md)
for the `match` rule syntax and
[ADR-0004](architecture/decisions/0004-unified-policy-engine-for-access-and-filtering.md)
for policy semantics.

```toml
[[access_policies]]
name = "db-reader"
[[access_policies.match]]
path = "$.role"              # JSONPath into the decoded claim set
pattern = "^db-reader$"      # regexp tested against the extracted value(s)
[[access_policies.grants]]
mcp = "postgres-ro"          # "*" grants every registered MCP
tools = ["*"]                # "*" grants every tool on the matched MCP(s)

[[access_policies]]
name = "db-writer"
[[access_policies.match]]
path = "$.role"
pattern = "^db-writer$"
[[access_policies.grants]]
mcp = "postgres-rw"
tools = ["*"]
```

**Fields:**
- `name` — unique identifier for the policy, used in error messages (required)
- `match` — a list of `{path, pattern}` rules (`[[access_policies.match]]`
  blocks), ANDed together (all must match)
- `grants` — a list of `{mcp, tools}` (`[[access_policies.grants]]` blocks); a
  call is authorized if this policy's `match` matches AND any grant covers the
  requested `(mcp, tool)`
- `enabled` — operator on/off switch (optional, default `true`). When
  `false`, the policy is skipped entirely during authorization, so it grants
  nothing. A caller authorized only by a disabled policy is rejected with
  `403 forbidden` and no downstream tool call is made — the pipeline never
  starts.

**How It Works:**
1. Gateway validates the JWT and decodes its full claim set
2. For each `access_policies` entry, `match`'s JSONPath rules are evaluated
   against the claims and regexp-tested; if a rule's path resolves to a list
   (e.g. `$.groups[*]`), it matches if *any* element satisfies the pattern
3. A call is authorized if **any** policy matches and grants the requested
   `(mcp, tool)` — policies are additive, there is no explicit deny yet
4. A malformed `path` or `pattern` is rejected by `Config.Validate()` at
   startup, not discovered per-request

### 6. Filter Policies

Strips response fields for a specific `(mcp, tool)` call when the caller's
claims match. Reuses the exact same `match` rule syntax as access policies —
see ADR-0004 for why one engine drives both.

```toml
[[filter_policies]]
name = "hide-pii-for-plain-users-get-user"
mcp = "postgres-ro"   # exact match, no "*" — a filter targets one tool
tool = "get_user"
drop_fields = ["$.hashed_password", "$.api_key", "$.internal_id"]
[[filter_policies.match]]
path = "$.role"
pattern = "^user$"
```

**Fields:**
- `name` — unique identifier (required)
- `match` — same `{path, pattern}` rule list as access policies
  (`[[filter_policies.match]]` blocks)
- `mcp` / `tool` — the exact tool call this filter applies to (no wildcards)
- `drop_fields` — JSONPath expressions identifying fields to remove, written
  against **the tool's own record** (`$.hashed_password`), not against the
  JSON-RPC envelope it travels in. The gateway unwraps that envelope and
  applies each path to every copy of the record it carries — MCP can return
  the same payload both as structured content and serialized into a text
  block, and both are filtered. See
  [ADR-0015](architecture/decisions/0015-filter-the-tool-payload-not-the-transport-envelope.md).
  A path that doesn't exist in a given response is a no-op, since
  `drop_fields` are authored once against a tool's general shape; the
  gateway logs a `WARN` when a policy matched a call and removed nothing.
  If a filter applies but the tool answered with free-form (non-JSON) text,
  the fields cannot be enforced and the call fails with
  `502 filter_unenforceable` rather than returning an unchecked body
- `enabled` — operator on/off switch (optional, default `true`). When
  `false`, the policy is skipped entirely: its `drop_fields` contribute
  nothing, so a response that this policy would have stripped is returned
  with all fields intact. Other enabled filter policies for the same
  `(mcp, tool)` still apply.

**How It Works:**
1. After a tool call succeeds, the gateway re-evaluates the caller's claims
   against `filter_policies`
2. The union of `drop_fields` from every matching policy scoped to that
   `(mcp, tool)` is removed from the response body before it's returned

## Complete Example

See [`config.example.toml`](../config.example.toml) for a complete configuration
file.

## Environment Variables

Environment-variable overrides are **not** currently supported — every setting
comes from the config file. (An earlier draft of this guide listed `GATEWAY_*`
variables; they were never implemented.)

## Best Practices

1. **Start Simple** — start with one or two access policies, add filter policies incrementally
2. **Test Policies** — verify access policies grant what you expect before layering filter policies on top
3. **Secure Secrets** — keep the config file readable only by the gateway user; use an external secret store for connection strings where possible
4. **Validate Config** — `Config.Validate()` compiles every policy's JSONPath/regexp up front; a malformed rule fails at startup, not at request time
5. **Monitor** — run at debug log level during initial rollout

## Troubleshooting

### "Access denied for a call I expected to be authorized"
- Check that the caller's JWT claims actually satisfy every rule in the
  policy's `match` list (all rules are ANDed)
- If `path` targets a list claim (e.g. group membership), confirm you used
  `$.groups[*]` (or the bare `$.groups` array form) — see ADR-0002
- Confirm a `grants` entry actually covers the specific `(mcp, tool)` pair
  being called, or uses `"*"` for the field(s) that should be wildcarded

### "MCP not found"
- Verify the MCP name in `grants` / `filter_policies` matches a name in a
  `[[mcps]]` block (or a runtime-registered MCP)

### "Field filtering not working"
- Ensure the filter policy's `mcp` / `tool` match exactly (no wildcards
  supported here, unlike access policy grants)
- Verify the caller's claims satisfy the filter policy's `match` rules
- Check that `drop_fields` paths match the actual response shape (a
  nonexistent path is a no-op, not an error — but the gateway logs
  `filter policy matched but removed no fields` when that happens, so check
  the log)
- Write paths against the tool's record (`$.hashed_password`), not against
  the transport envelope (`$.structuredContent.hashed_password`); the
  gateway unwraps the envelope itself (ADR-0015)

### Startup fails with "config: access_policies[N] ... match[M]: ..."
- The named policy's JSONPath expression or regexp failed to compile;
  fix the `path` / `pattern` at that index before restarting

### Startup fails with "config: parse ...: ..."
- The file is not valid TOML. A common cause is pasting an old YAML snippet;
  rewrite it in TOML (see the examples above and `config.example.toml`).

See `docs/DEBUG.md` for advanced debugging guidance.
