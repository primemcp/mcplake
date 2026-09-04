# MCP Gateway Configuration Guide

## Overview

The MCP Gateway is configured via YAML files. This guide covers all available configuration options.

## Configuration Sections

### 1. Server Configuration

Controls the gateway's HTTP server settings.

```yaml
server:
  port: 8080           # Main gateway port (default: 8080)
  ui_port: 8081        # Admin UI port (default: 8081)
  # tls:
  #   cert_file: /path/to/cert.pem
  #   key_file: /path/to/key.pem
  # read_timeout: 30s
  # write_timeout: 30s
```

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

### 3. MCP Servers

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

### 4. Routing Rules

Routes requests to specific MCP instances based on JWT claims.

```yaml
routing:
  - claim_key: role           # JWT claim to match
    claim_value: "db-reader"  # Expected value
    mcp_instance: postgres-ro # Target MCP instance
    priority: 10              # Matching priority (lower = higher priority)

  - claim_key: role
    claim_value: "db-writer"
    mcp_instance: postgres-rw
    priority: 10
```

**How It Works:**
1. Gateway extracts JWT and validates it
2. For each request, the router checks all rules
3. Rules are matched in priority order (lower number = checked first)
4. First matching rule determines the target MCP instance

### 5. Filtering Rules

Controls field-level response filtering based on JWT claims.

```yaml
filtering:
  - claim_key: role
    claim_value: "user"
    mcp_name: postgres-ro           # MCP to filter
    tool_name: get_user             # Specific tool (or "*" for all)
    hide_fields:
      - hashed_password
      - api_key
      - internal_id

  - claim_key: role
    claim_value: "user"
    mcp_name: postgres-ro
    tool_name: list_users
    hide_fields:
      - email
      - phone_number
```

**How It Works:**
1. After receiving a response from an MCP
2. The filter checks all rules for the caller's claims
3. Matching rules hide specified fields
4. Filtered response is returned to caller

## Complete Example

See `config.example.yaml` for a complete configuration file.

## Environment Variables

You can override configuration values with environment variables:

```bash
# Server
GATEWAY_SERVER_PORT=9000
GATEWAY_SERVER_UI_PORT=9001

# OIDC
GATEWAY_OIDC_PROVIDER_URL=https://auth.example.com
GATEWAY_OIDC_CLIENT_ID=my-gateway
GATEWAY_OIDC_AUDIENCE=my-gateway

# Debug logging
GATEWAY_LOG_LEVEL=debug
```

## Best Practices

1. **Start Simple** — Use a basic routing configuration first, add filtering rules incrementally
2. **Test Routes** — Verify routing rules before adding filtering
3. **Use Priorities** — Set distinct priorities to avoid ambiguous rules
4. **Secure Secrets** — Use environment variables or external secret management for sensitive values
5. **Validate Config** — Run with `--validate-config` to check for errors before deploying
6. **Monitor** — Enable debug logging during initial rollout

## Troubleshooting

### "No matching route"
- Check that the request's JWT claims match a routing rule
- Verify the `claim_key` and `claim_value` match exactly
- Check priorities (lower number = higher priority)

### "MCP instance not found"
- Verify the MCP instance name in the routing rule matches a defined MCP
- Check that the MCP is running and accessible

### "Field filtering not working"
- Ensure the filtering rule's `mcp_name` and `tool_name` match exactly
- Verify the caller's JWT claims match the filtering rule's `claim_key` and `claim_value`
- Check that the field names match the response schema

See `docs/DEBUG.md` for advanced debugging guidance.
