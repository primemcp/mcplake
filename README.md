# MCP Gateway

An open-source gateway for the Model Context Protocol (MCP) that enables secure, claims-based access control to MCP servers in air-gapped environments.

## Features

- **OIDC Authentication** — JWT-based authentication with claims validation
- **Claims-Based Routing** — Route requests to different MCP instances based on caller's JWT claims
- **Field-Level Response Filtering** — Strip response fields based on claims, not just tool-level access control
- **Schema Caching** — Efficient caching of tool and response schemas for minimal overhead
- **Live Pipeline Visualization** — Visual, graph-based UI showing real-time request routing and filtering decisions
- **Air-Gapped Ready** — Designed from the ground up for zero-egress environments

## Quick Start

### Prerequisites

- Go 1.27.1 or later
- An OIDC provider (for JWT issuance)
- Installed MCP servers

### Building

```bash
go build -o mcp-gateway ./cmd/gateway
```

### Running

```bash
./mcp-gateway --config config.yaml
```

## Architecture

The gateway consists of four main components:

1. **Auth Validator** — Verifies JWT signatures and validates claims
2. **Claims Router** — Routes requests to the appropriate MCP instance based on claims
3. **Schema Cache** — Caches tool and response schemas from all connected MCPs
4. **Response Filter** — Filters response fields based on caller's claims

See [docs/OVERVIEW.md](docs/OVERVIEW.md) for detailed architecture and design rationale.

## Phase 1 Roadmap

- [x] Project scaffolding and architecture design
- [ ] OIDC integration and JWT validation
- [ ] Claims-based routing engine
- [ ] Response filtering with schema inspection
- [ ] Web UI for pipeline visualization
- [ ] Configuration management
- [ ] Testing and CI/CD setup

## Configuration

Configuration is managed via YAML files. See `docs/CONFIG.md` (coming soon) for detailed configuration options.

## Contributing

This is an open-source project under Apache 2.0 license. Contributions are welcome.

To contribute:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Write tests
5. Open a pull request

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Community

- **Issues** — Report bugs or request features via GitHub Issues
- **Discussions** — Join community discussions
- **Design Docs** — See `docs/` for architecture and design rationale

## Acknowledgments

This project is inspired by the gap in the MCP gateway market for a tool that combines governance depth, deployment simplicity, and visual clarity.
