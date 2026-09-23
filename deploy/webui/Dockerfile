# Builds the admin UI as its own image: the SPA is compiled here, at image
# build time, and the running container only serves it. Nothing about the
# frontend toolchain survives into the final layer, and nothing builds when
# the container starts.
#
# Build context is the repo root; see compose.yaml.
#
# Why this exists separately from the gateway image (ADR-0020): the gateway
# still embeds its own copy of the UI for single-binary deployments, but
# coupling the frontend's build to the Go image means every UI change rebuilds
# the gateway, and puts a `bun install` in front of a Go toolchain that does
# not otherwise need one.

# ---- frontend: build the SPA (make ui-build's own steps) ----
FROM oven/bun:1 AS webui-build
WORKDIR /src
# package.json + lockfile first, on their own layer: dependencies change far
# less often than source does, so editing a component re-runs `bun run build`
# without re-running `bun install`.
COPY gateway/internal/controlplane/webui/package.json gateway/internal/controlplane/webui/bun.lock ./gateway/internal/controlplane/webui/
RUN cd gateway/internal/controlplane/webui && bun install --frozen-lockfile
# claimMatch.test.ts imports ../../../../../../testdata/claim_stringify_cases.json
# (kept alongside router/claimrule_test.go's Go twin of the same cases), so
# testdata/ has to land at the same relative depth from webui/ that it sits at
# in the real repo, not get flattened away like the rest of the repo.
COPY testdata ./testdata
COPY gateway/internal/controlplane/webui/ ./gateway/internal/controlplane/webui/
RUN cd gateway/internal/controlplane/webui && bun run build

# ---- server: the static file server + admin API proxy ----
FROM golang:1.27 AS server-build
WORKDIR /src
# cmd/webui is its own module in the workspace and depends on nothing outside
# the standard library, so only the workspace file and that module are needed
# here -- copying the whole repo would invalidate this layer on every backend
# change.
COPY go.work go.work.sum ./
COPY cmd/webui ./cmd/webui
# The workspace lists modules this stage deliberately does not copy, so build
# the module on its own rather than through go.work.
RUN cd cmd/webui && CGO_ENABLED=0 GOFLAGS=-mod=mod go build -o /out/webui-server .

# ---- runtime ----
# gcr.io/distroless/static: no shell, no package manager, no libc. This
# container is reachable from a browser and forwards to the admin API, so the
# smaller the thing behind that door, the better. :nonroot runs as uid 65532
# and the assets below are world-readable, so no chown is needed.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=webui-build /src/gateway/internal/controlplane/webui/dist /srv/www
COPY --from=server-build /out/webui-server /usr/local/bin/webui-server

EXPOSE 8081
# Defaults are baked in (see cmd/webui/main.go); compose overrides
# WEBUI_API_TARGET to point at the gateway's control plane.
ENTRYPOINT ["/usr/local/bin/webui-server"]
