# MCP Gateway — Architecture & Design Overview

## Table of Contents

1. [Vision & Goals](#vision--goals)
2. [Architecture](#architecture)
3. [Request Flow](#request-flow)
4. [Core Components](#core-components)
5. [Data Structures](#data-structures)
6. [Design Decisions](#design-decisions)

## Vision & Goals

The MCP Gateway is an open-source tool that sits between AI agents and their MCP servers, enabling secure, claims-based access control in air-gapped environments. Unlike existing solutions, it combines:

- **Real governance depth** — field-level response filtering, not just tool-level access control
- **Deployment simplicity** — single binary, zero external dependencies for production use
- **Visual clarity** — live graph UI showing exactly what the gateway is doing to each request

This is not a commercial product. Success is measured by adoption, production usage, and community contribution.

## Architecture

### High-Level Overview

```
┌─────────────┐
│  AI Agent   │
└──────┬──────┘
       │ 1. Login
       ▼
┌─────────────┐      ┌──────────────────────────────────────┐
│ OIDC        │      │         MCP Gateway                  │
│ Provider    │      │  ┌─────────────────────────────────┐ │
└──────┬──────┘      │  │ 3. Auth Validator (JWT)         │ │
       │             │  │    - Signature verification     │ │
       │ 2. JWT      │  │    - Claims validation          │ │
       │             │  └────────────┬────────────────────┘ │
       ▼             │               │                       │
   [Agent]           │  ┌────────────▼────────────────────┐ │
       │             │  │ 4. Claims Router                │ │
       │ 3. Request  │  │    - Route by claims            │ │
       │ + JWT       │  │    - Select MCP instance        │ │
       ▼             │  └────────────┬────────────────────┘ │
   ┌────────────┐    │               │                       │
   │  Gateway   │    │  ┌────────────▼────────────────────┐ │
   └────┬───────┘    │  │ Schema Cache                    │ │
        │            │  │ - Tool schemas                  │ │
        │            │  │ - Response schemas             │ │
        │            │  └────────────┬────────────────────┘ │
        │            │               │                       │
        │            │  ┌────────────▼────────────────────┐ │
        │            │  │ 5. Response Filter              │ │
        │            │  │    - Strip fields by claims     │ │
        │            │  │    - Redact sensitive data      │ │
        │            │  └────────────┬────────────────────┘ │
        │            │               │                       │
        │            │  ┌────────────▼────────────────────┐ │
        │            │  │ Admin/Graph UI                  │ │
        │            │  │ - Live pipeline visualization   │ │
        │            │  └────────────────────────────────┘ │
        │            └──────────────────────────────────────┘
        │
        ├─────────────┐
        │             │
        ▼             ▼
    ┌────────┐   ┌──────────┐   ┌────────┐
    │MCP #1  │   │  MCP #2  │   │MCP #3  │
    │read-   │   │read-write│   │other   │
    │only    │   │Postgres  │   │tool    │
    │Postgres│   │          │   │        │
    └────────┘   └──────────┘   └────────┘
```

### Component Responsibilities

**Auth Validator**
- Verifies JWT signature against OIDC provider's public key (cached JWKS)
- Validates standard claims (exp, iat, iss)
- Extracts custom claims for downstream use
- Returns 401 Unauthorized if validation fails

**Claims Router**
- Reads claims from validated JWT
- Maintains mapping of claims → MCP instances
- Routes each tool call to the correct instance
- Supports multiple instances of the same MCP (e.g., read-only and read-write Postgres)

**Schema Cache**
- Populated during gateway startup (one-time per MCP)
- Stores tool definitions and response schemas
- Enables the Response Filter to make field-level decisions
- Invalidated when MCPs are reloaded

**Response Filter**
- Uses schemas from the cache
- Removes fields from responses based on caller's claims
- Operates on JSON responses (future: pluggable filter chains)
- Maintains response validity (no partial objects, valid JSON)

**Admin/Graph UI**
- Web interface for gateway operators
- Shows live pipeline state (which claims → which MCP → which fields filtered)
- Allows MCPs to be registered/unregistered
- Displays audit logs (Phase 3)

## Request Flow

### Setup (One-Time)

1. Gateway starts and connects to all configured MCPs
2. For each MCP: calls `list_tools()` and captures response schemas
3. Stores schemas in in-memory cache with optional persistence

### Runtime (Per Request)

```
Client Request (with JWT)
        │
        ▼
[Auth Validator]
    │ valid? no  → 401 Unauthorized
    │ valid? yes
        │
        ▼
[Claims Router]
    │ reads claims.role = "db-writer"
    │ selects MCP #2 (read-write instance)
        │
        ▼
MCP #2 → [execute tool]
        │
        ▼
[Raw Response from MCP]
    │ e.g., {id: 123, email: "...", salary: 50000, hashed_pwd: "..."}
        │
        ▼
[Response Filter]
    │ claims say: hide salary, hashed_pwd
    │ uses schema to identify these fields
    │ removes them
        │
        ▼
[Filtered Response]
    │ {id: 123, email: "..."}
        │
        ▼
[Client]
```

## Core Components

### `gateway` Package

Main entry point and server orchestration.

```
cmd/
  gateway/
    main.go           # Entry point, flag parsing
config/
  config.go           # Configuration loading
  default.yaml        # Example configuration
auth/
  validator.go        # JWT validation
  claims.go           # Claims extraction
router/
  router.go           # Claims-based routing
  rules.go            # Routing rules
cache/
  schema.go           # Schema caching
filter/
  filter.go           # Response filtering
  rules.go            # Field filtering rules
ui/
  server.go           # Web server for UI
  handlers.go         # HTTP handlers
mcp/
  client.go           # MCP client connection
  transport.go        # MCP protocol transport
```

## Data Structures

### JWT Claims

```go
{
  "iss": "https://auth.example.com",
  "sub": "user@example.com",
  "aud": "mcp-gateway",
  "exp": 1234567890,
  "iat": 1234567890,
  
  // Custom claims
  "role": "db-writer",
  "org": "acme-corp",
  "team": "data-eng"
}
```

### Routing Rule

```go
type RoutingRule struct {
  ClaimKey    string  // e.g., "role"
  ClaimValue  string  // e.g., "db-writer"
  MCPInstance string  // e.g., "postgres-rw"
  Priority    int     // for multiple matching rules
}
```

### Field Filtering Rule

```go
type FieldFilterRule struct {
  ClaimKey    string    // e.g., "role"
  ClaimValue  string    // e.g., "user"
  MCPName     string    // e.g., "postgres"
  ToolName    string    // e.g., "get_user"
  HideFields  []string  // e.g., ["salary", "hashed_pwd"]
}
```

## Design Decisions

### Why Go?

- **Single Binary** — deployable with zero dependencies
- **Performance** — low-latency request processing (~1-2ms overhead target)
- **Concurrency** — goroutines for handling multiple concurrent requests
- **Operational Simplicity** — no runtime required, no version management

### Schema Caching

Instead of making field-level filtering decisions at runtime without schemas, the gateway:
1. Fetches schemas once at startup from each MCP
2. Caches them in memory (with optional persistence)
3. Uses cached schemas to make field-level filtering decisions

This trades one-time startup latency for much simpler downstream logic and better performance.

### Claims-Driven Everything

Rather than separate config files for routing, field filtering, and audit rules, everything flows from JWT claims:
- The same claims object drives routing decisions and filtering rules
- Policies are updated by changing how claims are issued (usually in your OIDC provider)
- This creates a single source of truth for "who gets what"

### Phase 1 Scope

The project focuses first on:
- OIDC + JWT validation
- Claims-based routing
- Field-level response filtering
- Visual UI

Later phases (argument validation, pluggable filters, audit trails) are explicitly deferred to keep Phase 1 achievable and demo-able.

### Air-Gapped Design

The gateway is designed to work in environments with zero outbound network access:
- OIDC public keys can be cached/pre-loaded
- MCP connections are inbound or local
- No telemetry or external calls required
- Optional: offline mode with static configuration

## Future Considerations

- **Argument Validation** (Phase 2) — validate tool arguments before sending to MCPs
- **Pluggable Filters** (Phase 2) — JS/Lua/Python/WASM-based transforms for requests and responses
- **Observability** (Phase 3) — audit logs, distributed tracing, metrics
- **Persistence** — optional schema persistence for faster restarts
- **Multi-Gateway Federation** — multiple gateway instances with shared state
