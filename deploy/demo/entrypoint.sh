#!/bin/sh
# app.New (cmd/gateway/app/app.go) fetches the OIDC JWKS and opens
# persistence synchronously at startup and fails hard if either isn't
# reachable yet. Compose starts containers, not "services are actually
# ready" - Keycloak's realm import and Postgres's own init scripts both take
# a few seconds after their process starts, so this waits for the real
# readiness signals (not just "the container is up") before exec'ing the
# gateway.
#
# It then preflights the demo MCP server, which app.New does *not* fail hard
# on -- see the comment above that check for why a demo needs it to.
set -eu

jwks_url="${GATEWAY_JWKS_URL:-http://localhost:8080/realms/mcplake/protocol/openid-connect/certs}"
pg_host="${GATEWAY_PG_HOST:-postgres}"
pg_port="${GATEWAY_PG_PORT:-5432}"
pg_mcp_host="${GATEWAY_PG_MCP_HOST:-localhost}"
pg_mcp_port="${GATEWAY_PG_MCP_PORT:-8000}"

echo "entrypoint: waiting for Keycloak realm ($jwks_url)..."
until curl --fail --silent --output /dev/null "$jwks_url"; do
    sleep 2
done
echo "entrypoint: Keycloak is ready"

echo "entrypoint: waiting for Postgres ($pg_host:$pg_port)..."
until pg_isready --host="$pg_host" --port="$pg_port" --quiet; do
    sleep 2
done
echo "entrypoint: Postgres is ready"

# The same idea applied to the third thing app.New depends on, which it does
# *not* fail hard on: the demo MCP server, a Python script the gateway execs
# as a subprocess (deploy/demo/config.toml's [[mcps]]).
#
# cache.Registry.Register deliberately tolerates an MCP it cannot reach - it
# files the registration away as "unreachable" and lets the gateway serve the
# others (ADR-0003). Correct for a real deployment, quietly useless for a
# demo whose only MCP is that one: the stack comes up, the admin UI lists the
# endpoint, and it has zero tools and does nothing.
#
# The way to get there is easy to hit. config.toml is a bind mount, so a
# `git pull` changes it immediately; python3 and fastmcp are baked into the
# image, so they only change on a rebuild. Run `compose up -d` without
# `--build` after pulling a revision that switched the demo MCP and the two
# halves disagree. Fail here instead, where the reason can be named.
demo_mcp_script="${GATEWAY_DEMO_MCP_SCRIPT:-/opt/mcplake/demo-mcp/employee_directory.py}"

fail() {
    echo "entrypoint: $1" >&2
    echo "entrypoint: $2" >&2
    echo "entrypoint: refusing to start - the gateway would come up with an" >&2
    echo "entrypoint: unreachable MCP and no tools. See docs/DEMO.md." >&2
    exit 1
}

stale_image() {
    fail "$1" "Rebuild the image: 'docker compose up -d --build' (or 'podman compose up -d --build')."
}

command -v python3 >/dev/null 2>&1 ||
    stale_image "python3 is not installed in this image."
python3 -c 'import fastmcp' >/dev/null 2>&1 ||
    stale_image "fastmcp is not importable by this image's python3."
[ -f "$demo_mcp_script" ] ||
    fail "the demo MCP server is missing at $demo_mcp_script." \
         "compose.yaml mounts deploy/demo/mcp-servers there - check that mount."
echo "entrypoint: demo MCP server is present and its dependencies import"

# And the same again for the sse MCP in its own container -- though since
# ADR-0019 this one is a convenience rather than a correctness requirement.
#
# The gateway now pings every registered MCP on mcp.health_check_interval and
# reconnects the ones that are not answering, so an MCP that is merely slow to
# start is picked up within half a minute on its own. What this wait buys is
# that the demo is fully working the moment compose reports it up, instead of
# spending its first half-minute with one of its two MCPs showing
# "unreachable" and no tools -- which reads like the #180 failure even though
# it would resolve itself.
#
# A TCP connect rather than an HTTP request: the endpoint is an SSE stream
# that stays open once accepted, so curl would hang rather than return, and
# "something is listening" is all this needs to know.
echo "entrypoint: waiting for the Postgres MCP ($pg_mcp_host:$pg_mcp_port)..."
until python3 - "$pg_mcp_host" "$pg_mcp_port" <<'PY' 2>/dev/null
import socket, sys
socket.create_connection((sys.argv[1], int(sys.argv[2])), timeout=2).close()
PY
do
    sleep 2
done
echo "entrypoint: the Postgres MCP is accepting connections"

exec mcp-gateway "$@"
