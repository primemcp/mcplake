#!/bin/sh
# app.New (cmd/gateway/app/app.go) fetches the OIDC JWKS and opens
# persistence synchronously at startup and fails hard if either isn't
# reachable yet. Compose starts containers, not "services are actually
# ready" - Keycloak's realm import and Postgres's own init scripts both take
# a few seconds after their process starts, so this waits for the real
# readiness signals (not just "the container is up") before exec'ing the
# gateway.
set -eu

jwks_url="${GATEWAY_JWKS_URL:-http://localhost:8080/realms/mcplake/protocol/openid-connect/certs}"
pg_host="${GATEWAY_PG_HOST:-postgres}"
pg_port="${GATEWAY_PG_PORT:-5432}"

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

exec mcp-gateway "$@"
