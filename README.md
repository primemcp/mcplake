# MCP Gateway

An open-source gateway for the Model Context Protocol (MCP) that enables secure, claims-based access control to MCP servers in air-gapped environments.

## Features

- **OIDC Authentication** — JWT-based authentication with claims validation
- **Claims-Based Routing** — Route requests to different MCP instances based on caller's JWT claims
- **Field-Level Response Filtering** — Strip response fields based on claims, not just tool-level access control
- **Schema Caching** — Efficient caching of tool and response schemas for minimal overhead
- **Authenticated Admin API** — Control-plane `/admin/*` gated by a Bearer JWT plus config-file claim rules (`admin_auth`)
- **MCP Control Server** — Optionally expose every admin operation as MCP tools, so an MCP client can operate the gateway (`admin_mcp`)
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
./mcp-gateway --config config.toml
```

## Architecture

The gateway consists of four main components:

1. **Auth Validator** — Verifies JWT signatures and validates claims
2. **Claims Router** — Routes requests to the appropriate MCP instance based on claims
3. **Schema Cache** — Caches tool and response schemas from all connected MCPs
4. **Response Filter** — Filters response fields based on caller's claims

The gateway speaks MCP on the way in as well as on the way out: point an MCP
client at `/v1/mcp` (Streamable HTTP) or `/v1/sse` and it sees a tool catalogue
filtered to exactly what its token authorizes, with every tool named
`<mcp>__<tool>`. `POST /v1/call` remains available for plain REST callers; both
run the same pipeline. See [docs/api/data-plane.md](docs/api/data-plane.md).

See [docs/architecture/overview.rst](docs/architecture/overview.rst) for the architecture, and the [decision records](docs/architecture/decisions/) for design rationale.

## Phase 1 Roadmap

- [x] Project scaffolding and architecture design
- [x] fasthttp data-plane gateway (OIDC/JWT validation, claims-based authorization, tool-call proxy)
- [x] MCP endpoints on the data plane — connect any MCP client to the gateway over Streamable HTTP (`/v1/mcp`) or SSE (`/v1/sse`), with `tools/list` filtered per caller ([ADR-0021](docs/architecture/decisions/0021-serve-mcp-on-the-data-plane.rst))
- [x] Dynamic MCP registration and schema discovery
- [x] Unified JSONPath+regexp claim-rule engine (access policies and response filtering)
- [x] GORM persistence (SQLite default, PostgreSQL-ready)
- [x] Gin control-plane admin API with OpenAPI/Swagger docs
- [ ] Web UI for pipeline visualization
- [ ] Testing and CI/CD setup

## Configuration

Configuration is a single TOML file ([ADR-0012](docs/architecture/decisions/0012-toml-configuration-format.rst)), passed with `--config` (default `config.toml`). See [`config.example.toml`](config.example.toml) and [`docs/CONFIG.md`](docs/CONFIG.md) for all options.

## Demo

Want to see it running end to end before wiring up your own OIDC provider
and MCP servers? `docker compose up --build` (or `podman compose up --build`)
brings up Keycloak, Postgres, MCP Inspector, and **two** MCP servers — one a
stdio subprocess, one a real server in its own container reached over `sse` —
with demo users that show a `200` vs a `403` under the same claims-based
access control the gateway uses in production, a live field-level filtering
example, and a per-MCP grant that lets one user reach one server and not the
other. See [`docs/DEMO.md`](docs/DEMO.md),
including [Upgrading or resetting the demo](docs/DEMO.md#upgrading-or-resetting-the-demo)
if you are coming back to it after pulling a newer revision.

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
