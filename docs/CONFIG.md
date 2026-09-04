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
  provider_url: "https://auth.example.com"
  client_id: "mcp-gateway"
  audience: "mcp-gateway"
  # jwks_cache_ttl: 1h
  # offline_mode: false
  # offline_jwks_file: /etc/mcp-gateway/jwks.json
```

**Fields:**
- `provider_url` — URL of your OIDC provider (required)
- `client_id` — Client ID from your OIDC provider (required)
- `audience` — Expected audience claim in JWT (required)
- `jwks_cache_ttl` — How long to cache JWKS (default: 1h)
- `offline_mode` — Use pre-loaded JWKS instead of fetching (default: false)
- `offline_jwks_file` — Path to cached JWKS for offline mode

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
