# Scaffolding Guide

This document describes the Go 1.27.1 workspace project structure and provides guidance for implementing the gateway.

## Project Structure

This is a **multi-module Go workspace** using `go.work` (Go 1.18+). Each library and the application have independent Go modules with their own dependencies.

```
.
├── go.work                       # Workspace definition (development environment)
├── cmd/
│   └── gateway/
│       ├── go.mod              # Application module
│       ├── go.sum
│       └── main.go             # Entry point, flag parsing, orchestration
├── auth/
│   ├── go.mod                  # Auth module
│   ├── go.sum
│   ├── validator.go            # JWT validation, OIDC integration
│   └── validator_test.go       # Tests for auth validator
├── router/
│   ├── go.mod                  # Router module
│   ├── go.sum
│   ├── router.go               # Claims-based routing
│   └── router_test.go          # Tests for router
├── cache/
│   ├── go.mod                  # Cache module
│   ├── go.sum
│   └── schema.go               # Schema caching from MCPs
├── filter/
│   ├── go.mod                  # Filter module
│   ├── go.sum
│   ├── filter.go               # Response field filtering
│   └── filter_test.go          # Tests for filter
├── mcp/
│   ├── go.mod                  # MCP protocol module
│   ├── go.sum
│   └── client.go               # MCP client protocol implementation
├── config/
│   ├── go.mod                  # Config module
│   ├── go.sum
│   └── config.go               # Configuration loading and validation
├── gateway/
│   ├── go.mod                  # Gateway core module
│   ├── go.sum
│   └── internal/
│       └── gateway.go          # Main gateway orchestration (internal)
├── docs/
│   ├── OVERVIEW.md             # Architecture and design overview
│   └── CONFIG.md               # Configuration guide
├── README.md                    # User-facing project overview
├── CONTRIBUTING.md             # Contribution guidelines
├── LICENSE                      # Apache 2.0 license
├── Makefile                     # Build and test commands
├── config.example.toml          # Example configuration
└── .gitignore                   # Git ignore patterns
```

## Module Independence

Each module is a standalone Go module that can be developed and tested independently:

```bash
# Test a single module outside the workspace
cd auth
GOWORK=off go test ./...
```

This ensures that modules don't accidentally depend on the workspace and can be used independently if needed.

## Implementation Order (Recommended)

### Phase 1: Core Infrastructure

1. **Config Module** (`config/config.go`)
   - Parse TOML configuration
   - Validate required fields
   - Provide sensible defaults

2. **Auth Module** (`auth/validator.go`)
   - Fetch JWKS from OIDC provider
   - Cache with TTL
   - Verify JWT signatures
   - Extract claims

3. **MCP Module** (`mcp/client.go`)
   - Establish MCP connections (stdio transport first)
   - Handle list_tools() method
   - Call tool execution
   - Connection lifecycle management

### Phase 2: Gateway Logic

4. **Cache Module** (`cache/schema.go`)
   - Populate from MCP clients at startup
   - Store tool and response schemas
   - Provide efficient lookups
   - Support cache invalidation

5. **Router Module** (`router/router.go`)
   - Implement claims-based routing logic
   - Match routing rules
   - Handle priority selection
   - Return target MCP instance

6. **Filter Module** (`filter/filter.go`)
   - Load filtering rules
   - Match rules by claims
   - Use schema cache to identify fields
   - Strip fields from JSON responses

### Phase 3: Gateway Server

7. **Gateway Module** (`gateway/internal/gateway.go`)
   - Wire all components together
   - Implement HTTP request handlers
   - Orchestrate auth → routing → MCP call → filtering
   - Error handling and logging

8. **Command** (`cmd/gateway/main.go`)
   - Parse command-line flags
   - Initialize all modules
   - Start HTTP server
   - Handle graceful shutdown

### Phase 4: UI (Later)

- Web-based admin interface
- Live pipeline visualization
- Configuration management UI

## Key Implementation Notes

### JWT Claims Structure

Expect JWT claims like:

```json
{
  "iss": "https://auth.example.com",
  "sub": "user@example.com",
  "aud": "mcp-gateway",
  "exp": 1234567890,
  "iat": 1234567890,
  "role": "db-writer",
  "org": "acme",
  "team": "data-eng"
}
```

Custom claims (role, org, team, etc.) drive routing and filtering decisions.

### Error Handling

- 400 Bad Request — Invalid request format
- 401 Unauthorized — Invalid or missing JWT
- 403 Forbidden — Claims don't authorize this action
- 404 Not Found — MCP instance or tool not found
- 500 Internal Server Error — Gateway error

### Concurrency

- Use goroutines for concurrent MCP calls
- Protect schema cache with sync.RWMutex
- Use context.Context for cancellation

### Testing Strategy

- Unit tests for each module
- Mock interfaces for external dependencies
- Integration tests for module interactions
- Use Testify for assertions and mocking
- TDD: write tests before implementation

## Dependencies by Module

### auth
- `github.com/golang-jwt/jwt/v5` — JWT parsing and validation

### config
- `github.com/BurntSushi/toml` — TOML configuration parsing (ADR-0012)

### gateway
- All other modules as internal dependencies
- Standard library http and net packages

### Other modules
- Minimal external dependencies
- Standard library focus

## Building & Running

Requires Go 1.27.1+

```bash
# Sync workspace (required after adding/removing modules)
go work sync

# Build the application
make build

# Run with example config
./cmd/gateway/mcp-gateway --config config.example.toml

# Run all tests
make test

# Run tests with race detector
make test-race

# Format code
make fmt

# Lint code
make lint
```

## Workspace Maintenance

When adding a new module, update `go.work`:

```bash
go work use ./new-module
go work sync
```

When removing a module, update `go.work` manually by editing it.

Commit both `go.work` and each module's `go.mod` and `go.sum` files.

## Next Steps

1. Implement `config.Load()` to parse TOML configuration
2. Implement `auth.Validator` with OIDC provider integration  
3. Implement `mcp.Client` and MCP transport
4. Build schema caching in `cache` module
5. Implement routing logic in `router` module
6. Implement filtering in `filter` module
7. Wire components in `gateway` module
8. Integrate into `cmd/gateway` main function
9. Add HTTP server startup

See [docs/architecture/design-history.rst](docs/architecture/design-history.rst) for detailed architecture decisions and design rationale.
