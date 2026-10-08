# Builds the demo image for the mcplake gateway: the admin UI, the Go
# binary, and the employee-directory MCP server it execs as a subprocess.
# That one ships inside this image because it is a *stdio* MCP and stdio
# means a subprocess - not because the gateway cannot reach an MCP
# elsewhere. Since ADR-0017 it can, and the demo's other MCP
# (postgres-mcp in compose.yaml) is its own container with its own image.
# Build context is the repo root; see compose.yaml.

# ---- frontend: build the admin UI (make ui-build's own steps) ----
FROM oven/bun:1 AS webui-build
WORKDIR /src
COPY gateway/internal/controlplane/webui/package.json gateway/internal/controlplane/webui/bun.lock ./gateway/internal/controlplane/webui/
RUN cd gateway/internal/controlplane/webui && bun install --frozen-lockfile
# claimMatch.test.ts imports ../../../../../../testdata/claim_stringify_cases.json
# (kept alongside router/claimrule_test.go's Go twin of the same cases), so
# testdata/ has to land at the same relative depth from webui/ that it sits
# at in the real repo, not get flattened away like the rest of the repo.
COPY testdata ./testdata
COPY gateway/internal/controlplane/webui/ ./gateway/internal/controlplane/webui/
RUN cd gateway/internal/controlplane/webui && bun run build

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
COPY --from=webui-build /src/gateway/internal/controlplane/webui/dist ./gateway/internal/controlplane/webui/dist
# glebarez/sqlite is pure Go (modernc.org/sqlite); CGO_ENABLED=0 keeps the
# binary portable across the build and runtime base images.
RUN CGO_ENABLED=0 go build -o /out/mcp-gateway ./cmd/gateway

# ---- runtime ----
# python:*-slim (not a Go-only distroless image) because the dummy MCP
# server is a FastMCP script the gateway execs as a subprocess.
FROM python:3.12-slim-bookworm
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl postgresql-client \
    && rm -rf /var/lib/apt/lists/*
# Installed at build time: this project's whole premise is air-gapped
# operation (README), so the demo shouldn't need network access every time
# the container starts.
COPY deploy/demo/mcp-servers/requirements.txt /opt/mcplake/demo-mcp/requirements.txt
RUN pip install --no-cache-dir -r /opt/mcplake/demo-mcp/requirements.txt
# Baked in so the image runs on its own, but compose.yaml also bind-mounts
# deploy/demo/mcp-servers over this directory: the script is then editable
# without a rebuild, and can never be a revision behind the config.toml that
# names it (also a mount). The interpreter and fastmcp above are the parts
# that genuinely need a rebuild -- entrypoint.sh checks for them and says so
# rather than letting the gateway boot with an unreachable MCP.
COPY deploy/demo/mcp-servers/employee_directory.py /opt/mcplake/demo-mcp/employee_directory.py
COPY --from=build /out/mcp-gateway /usr/local/bin/mcp-gateway
COPY deploy/demo/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
