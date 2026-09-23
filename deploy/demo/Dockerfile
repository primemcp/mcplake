# Builds the demo image for the mcplake gateway: the admin UI, the Go
# binary, and the dummy MCP server it execs as a subprocess (mcp.Client only
# speaks stdio - ADR-0003 - so the MCP server has to live in this image, not
# in its own compose service). Build context is the repo root; see
# compose.yaml.

# ---- frontend: build the admin UI (make ui-build's own steps) ----
FROM oven/bun:1 AS webui-build
WORKDIR /src
COPY gateway/internal/controlplane/webui/package.json gateway/internal/controlplane/webui/bun.lock ./
RUN bun install --frozen-lockfile
COPY gateway/internal/controlplane/webui/ ./
RUN bun run build

# ---- backend: build the gateway binary against the full workspace ----
FROM golang:1.27 AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY auth ./auth
COPY cache ./cache
COPY cmd ./cmd
COPY config ./config
COPY filter ./filter
COPY gateway ./gateway
COPY mcp ./mcp
COPY persistence ./persistence
COPY router ./router
COPY --from=webui-build /src/dist ./gateway/internal/controlplane/webui/dist
# glebarez/sqlite is pure Go (modernc.org/sqlite); CGO_ENABLED=0 keeps the
# binary portable across the build and runtime base images.
RUN CGO_ENABLED=0 go build -o /out/mcp-gateway ./cmd/gateway

# ---- runtime ----
# node:*-slim (not a Go-only distroless image) because the dummy MCP server
# is an npm package the gateway execs as a subprocess.
FROM node:22-bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl postgresql-client \
    && rm -rf /var/lib/apt/lists/*
# Installed at build time, not npx'd at run time: this project's whole
# premise is air-gapped operation (README), so the demo shouldn't need
# network access to the npm registry every time the container starts. This
# also gives the "mcp-server-postgres" binary name config.example.toml
# already assumes is directly on PATH.
RUN npm install -g @modelcontextprotocol/server-postgres
COPY --from=build /out/mcp-gateway /usr/local/bin/mcp-gateway
COPY deploy/demo/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
