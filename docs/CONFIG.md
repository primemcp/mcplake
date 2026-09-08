# MCP Gateway Configuration Guide

## Overview

The MCP Gateway is configured via YAML files. This guide covers all available configuration options.

## Configuration Sections

### 1. Server Configuration

Controls the gateway's two HTTP surfaces — see
[`docs/architecture/overview.md`](architecture/overview.md) for why they're
separate.

```yaml
server:
  data_plane_addr: ":8080"    # fasthttp tool-call proxy (ADR-0001)
  control_plane_addr: ":8081" # Gin admin API (ADR-0005)
  # tls:
  #   cert_file: /path/to/cert.pem
  #   key_file: /path/to/key.pem
  # read_timeout: 30s
  # write_timeout: 30s
```

**Fields:**
- `data_plane_addr` — listen address for the tool-call endpoint used by agents
  (`POST /v1/call`, `GET /healthz`). Required.
- `control_plane_addr` — listen address for the admin API used to manage MCP
  registrations and policies (`/admin/*`). Bind this to a trusted network/interface
  only — see [ADR-0005](architecture/decisions/0005-use-gin-for-control-plane-api.md).

### 2. OIDC Configuration

Configures JWT validation and OIDC provider integration.

```yaml
oidc:
  jwks_url: "https://auth.example.com/.well-known/jwks.json"
  issuer: "https://auth.example.com"
  audience: "mcp-gateway"
  # jwks_cache_ttl: 1h
```

**Fields:**
- `jwks_url` — the OIDC provider's JWKS endpoint (required). The gateway
  currently requires this exact URL; it does not yet perform OIDC discovery
  from a provider/issuer URL (`/.well-known/openid-configuration`) — that's
  tracked as future work, not implemented in `auth.Validator` yet.
- `issuer` — required `iss` claim value (required)
- `audience` — required `aud` claim value (required)
- `jwks_cache_ttl` — how long fetched keys are cached before a background
  refresh (default: `1h`). The cache is also refreshed out-of-band, rate-limited
  to once per minute, whenever a token references an unrecognized key ID
  (e.g. right after the provider rotates its signing key) — see
  [`auth.Validator`](../auth/validator.go).

Every JWT validation failure (bad signature, expired, wrong `iss`/`aud`,
malformed token) is reported the same way — as `auth.ErrUnauthorized` — and
maps to a 401 at the gateway's tool-call endpoint.

### 3. Persistence

Configures the control-plane's durable store for MCP registrations and
access/filter policies. See
[ADR-0006](architecture/decisions/0006-gorm-sqlite-postgres-persistence.md).

```yaml
persistence:
  driver: sqlite       # sqlite (default) | postgres
  dsn: "gateway.db"     # SQLite file path, or a Postgres connection string
```

**Fields:**
- `driver` — `sqlite` (default) or `postgres`. SQLite runs embedded via a
  pure-Go driver (no CGO, no external service) — the default for a
  single-instance, air-gapped deployment. `postgres` is for distributed
  deployments sharing one control-plane store.
- `dsn` — the SQLite file path when `driver: sqlite`, or a PostgreSQL
  connection string when `driver: postgres`.

At startup, this section's entries plus `mcps:`/`access_policies:`/
`filter_policies:` below are upserted into this store by name — the database
becomes the source of truth from then on; `config.yaml` is a seed mechanism,
not a parallel state store.

### 4. MCP Servers

Defines all MCP servers the gateway connects to.

```yaml
mcps:
  - name: postgres-ro
    type: stdio               # stdio, sse, etc.
    command: mcp-server-postgres
    arguments:
      - "--connection-string"
      - "postgresql://user@localhost/db"
      - "--read-only"

  - name: filesystem
    type: stdio
    command: mcp-server-filesystem
    arguments:
      - "/data"
    # env:
    #   LOG_LEVEL: debug
```

**Fields:**
- `name` — Unique identifier for this MCP instance (required)
- `type` — Transport type (default: "stdio")
- `command` — Binary to execute (required)
- `arguments` — Command-line arguments
- `env` — Environment variables
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

```yaml
access_policies:
  - name: db-reader
    match:
      - path: "$.role"       # JSONPath into the decoded claim set
        pattern: "^db-reader$" # regexp tested against the extracted value(s)
    grants:
      - mcp: postgres-ro      # "*" grants every registered MCP
        tools: ["*"]          # "*" grants every tool on the matched MCP(s)

  - name: db-writer
    match:
      - path: "$.role"
        pattern: "^db-writer$"
    grants:
      - mcp: postgres-rw
        tools: ["*"]
```

**Fields:**
- `name` — unique identifier for the policy, used in error messages (required)
- `match` — a list of `{path, pattern}` rules, ANDed together (all must match)
- `grants` — a list of `{mcp, tools}`; a call is authorized if this policy's
  `match` matches AND any grant covers the requested `(mcp, tool)`
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

```yaml
filter_policies:
  - name: hide-pii-for-plain-users-get-user
    match:
      - path: "$.role"
        pattern: "^user$"
    mcp: postgres-ro   # exact match, no "*" — a filter targets one tool
    tool: get_user
    drop_fields:
      - "$.hashed_password" # JSONPath into the tool's response body
      - "$.api_key"
      - "$.internal_id"
```

**Fields:**
- `name` — unique identifier (required)
- `match` — same `{path, pattern}` rule list as access policies
- `mcp` / `tool` — the exact tool call this filter applies to (no wildcards)
- `drop_fields` — JSONPath expressions identifying fields to remove from the
  response; a path that doesn't exist in a given response is a no-op
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

See `config.example.yaml` for a complete configuration file.

## Environment Variables

You can override configuration values with environment variables:

```bash
# Server
GATEWAY_SERVER_DATA_PLANE_ADDR=:9000
GATEWAY_SERVER_CONTROL_PLANE_ADDR=:9001

# OIDC
GATEWAY_OIDC_PROVIDER_URL=https://auth.example.com
GATEWAY_OIDC_CLIENT_ID=my-gateway
GATEWAY_OIDC_AUDIENCE=my-gateway

# Debug logging
GATEWAY_LOG_LEVEL=debug
```

## Best Practices

1. **Start Simple** — start with one or two access policies, add filter policies incrementally
2. **Test Policies** — verify access policies grant what you expect before layering filter policies on top
3. **Secure Secrets** — use environment variables or external secret management for sensitive values
4. **Validate Config** — `Config.Validate()` compiles every policy's JSONPath/regexp up front; a malformed rule fails at startup, not at request time
5. **Monitor** — enable debug logging during initial rollout

## Troubleshooting

### "Access denied for a call I expected to be authorized"
- Check that the caller's JWT claims actually satisfy every rule in the
  policy's `match` list (all rules are ANDed)
- If `path` targets a list claim (e.g. group membership), confirm you used
  `$.groups[*]` (or the bare `$.groups` array form) — see ADR-0002
- Confirm a `grants` entry actually covers the specific `(mcp, tool)` pair
  being called, or uses `"*"` for the field(s) that should be wildcarded

### "MCP not found"
- Verify the MCP name in `grants`/`filter_policies` matches a name under
  `mcps:` (or a runtime-registered MCP, once the admin API exists)

### "Field filtering not working"
- Ensure the filter policy's `mcp`/`tool` match exactly (no wildcards
  supported here, unlike access policy grants)
- Verify the caller's claims satisfy the filter policy's `match` rules
- Check that `drop_fields` paths match the actual response shape (a
  nonexistent path is silently a no-op, not an error)

### Startup fails with "config: access_policies[N] ... match[M]: ..."
- The named policy's JSONPath expression or regexp failed to compile;
  fix the `path`/`pattern` at that index before restarting

See `docs/DEBUG.md` for advanced debugging guidance.
