#!/usr/bin/env bash
# Shared helpers for the pipeline recordings under docs/media/pipeline/.
# Sourced by every .tape, never run standalone -- keeps the commands a
# viewer actually sees typed on screen short and about the gateway's
# pipeline, not about fetching a token. Mirrors docs/DEMO.md's own curl
# snippets exactly, so a recording never shows a shortcut the walkthrough
# doesn't.
set -euo pipefail

KEYCLOAK_TOKEN_URL="http://localhost:8080/realms/mcplake/protocol/openid-connect/token"
GATEWAY_URL="http://localhost:9090/v1/call"
ADMIN_API_URL="http://localhost:9091"

# token_for USERNAME -- the demo's three users all have password == username
# (alice/alice, bob/bob, mcplake-admin/mcplake-admin; see docs/DEMO.md).
token_for() {
  curl -s -X POST "$KEYCLOAK_TOKEN_URL" \
    -d grant_type=password \
    -d client_id=mcplake-demo \
    -d username="$1" \
    -d password="$1" \
    -d scope=openid | jq -r .access_token
}

# role_claim_of TOKEN -- decodes the JWT payload's `role` claim, no
# signature check (same as the admin UI's own decodeJwtPayload -- this is
# for a human to read on screen, not for anything the gateway trusts).
role_claim_of() {
  jq -R 'split(".") | .[1] | @base64d | fromjson | .role' <<<"$1"
}

# call_gateway TOKEN MCP TOOL [JSON_ARGS]
call_gateway() {
  local token="$1" mcp="$2" tool="$3" args="${4:-{\}}"
  curl -s -o /tmp/mcplake-pipeline-body.json -w '%{http_code}' -X POST "$GATEWAY_URL" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    -d "{\"mcp\":\"$mcp\",\"tool\":\"$tool\",\"arguments\":$args}"
}

# show_last_call -- prints the status curl -w saved plus the pretty body,
# the recording's actual payoff shot.
show_last_call() {
  local status="$1"
  echo "HTTP $status"
  jq . /tmp/mcplake-pipeline-body.json
}

# set_enabled TOKEN MCP_NAME true|false -- PATCH /admin/mcps/:name.
set_enabled() {
  curl -s -X PATCH "$ADMIN_API_URL/admin/mcps/$2" \
    -H "Authorization: Bearer $1" \
    -H "Content-Type: application/json" \
    -d "{\"enabled\":$3}" | jq '{name, enabled, status}'
}
