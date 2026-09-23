# Demo: Compose Stack

A turnkey stack that brings up the gateway end to end - Keycloak as the OIDC
provider, a dummy MCP server, and Postgres - with one command and no manual
setup. See [`compose.yaml`](../compose.yaml) for the services and
[`config.example.toml`](../config.example.toml)/[`CONFIG.md`](CONFIG.md) for
what a real deployment's config file looks like; this demo's own config is
[`deploy/demo/config.toml`](../deploy/demo/config.toml).

## What it stands up

| Service    | Role                                                                                   |
|------------|-----------------------------------------------------------------------------------------|
| `keycloak` | OIDC provider. A realm (`mcplake`) is pre-imported with a client and two demo users.     |
| `gateway`  | The mcplake gateway, built from this repo, fronting the dummy MCP below.                 |
| `postgres` | Backs the gateway's own `[persistence]` (a `gateway` database) *and* the dummy MCP's data (a separate `demo` database). |

The dummy MCP is the [official postgres reference server](https://github.com/modelcontextprotocol/servers-archived/tree/main/src/postgres),
run as a subprocess inside the gateway's own container - `mcp.Client`
(`mcp/client.go`) only speaks stdio today (ADR-0003), so a downstream MCP
can't be a separate compose service the way Keycloak and Postgres are. It
exposes one tool, `query`, and enforces read-only queries itself.

Two demo users exercise claims-based access control (ADR-0002/ADR-0004)
end to end:

- **alice** (`role=db-reader`) is granted access to `postgres-demo` by the
  `demo-reader` access policy → her calls succeed.
- **bob** (`role=guest`) authenticates fine but matches no access policy →
  his calls get `403 forbidden`.

## Prerequisites

- Docker with the Compose plugin (`docker compose`), or Podman with
  `podman compose` (either works from the same `compose.yaml` - the demo
  intentionally avoids anything docker-specific like BuildKit-only syntax).
- `curl` and `jq` (`jq` is only used below to pull `access_token` out of the
  response; skip it and read the JSON by eye if you don't have it).

## Running it

```bash
docker compose up --build
# or: podman compose up --build
```

First boot pulls the Keycloak/Postgres images, builds the gateway image
(admin UI + Go binary), and imports the realm - give it a minute or two. The
gateway's own entrypoint (`deploy/demo/entrypoint.sh`) waits for Keycloak's
realm and Postgres to actually be ready before starting, so a clean exit
with no gateway errors means the stack is up.

Leave it running in that terminal, or add `-d` to run detached.

## Trying it out

Get a token for **alice** (`role=db-reader`):

```bash
ALICE_TOKEN=$(curl -s -X POST http://localhost:8080/realms/mcplake/protocol/openid-connect/token \
  -d grant_type=password \
  -d client_id=mcplake-demo \
  -d username=alice \
  -d password=alice \
  -d scope=openid | jq -r .access_token)
```

Call the dummy MCP through the gateway's data plane:

```bash
curl -s -X POST http://localhost:9090/v1/call \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mcp":"postgres-demo","tool":"query","arguments":{"sql":"select * from widgets"}}' | jq
```

This returns `200` with the seeded `widgets` rows
(`deploy/demo/postgres/02-demo-seed.sql`).

Now the same thing for **bob** (`role=guest`):

```bash
BOB_TOKEN=$(curl -s -X POST http://localhost:8080/realms/mcplake/protocol/openid-connect/token \
  -d grant_type=password \
  -d client_id=mcplake-demo \
  -d username=bob \
  -d password=bob \
  -d scope=openid | jq -r .access_token)

curl -s -o /dev/null -w '%{http_code}\n' -X POST http://localhost:9090/v1/call \
  -H "Authorization: Bearer $BOB_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mcp":"postgres-demo","tool":"query","arguments":{"sql":"select * from widgets"}}'
```

This prints `403` - bob authenticated (Keycloak issued him a token) but no
`[[access_policies]]` entry grants his `role=guest` claim access to
`postgres-demo`.

`GET /healthz` on the data plane needs no token:

```bash
curl -s http://localhost:9090/healthz
```

## What's intentionally not in this demo

- **The control-plane admin API isn't published to the host.**
  `control_plane_addr = "127.0.0.1:9091"` in `deploy/demo/config.toml`
  matches the project's own secure default (see `README.md` and
  [ADR-0005](architecture/decisions/0005-use-gin-for-control-plane-api.md)) -
  registering an MCP means starting a process on that host, so it stays on
  loopback until an operator explicitly configures `admin_auth` and widens
  it. Reach it from inside the shared network namespace if you need it:
  `docker compose exec keycloak curl http://localhost:9091/healthz` (the
  gateway container shares Keycloak's network namespace - see the note
  below).
- **No `filter_policies` example.** The postgres MCP server returns query
  results as a JSON-encoded string inside `content[].text`, not structured
  JSON - there's no per-field shape for a JSONPath-based filter
  (`filter.Strip`) to reach into, so a filter-policy demo here would just be
  misleading. See [ADR-0015](architecture/decisions/0015-filter-the-tool-payload-not-the-transport-envelope.md)
  for what filtering actually targets.
- **No writable MCP.** The reference postgres server only ever runs
  read-only queries; there's no upstream `postgres-rw` counterpart to demo
  (unlike the illustrative, non-literal example in `config.example.toml`).

## Why the gateway shares Keycloak's network namespace

`config.validateSecureHTTPURL` (`config/config.go`) requires `oidc.jwks_url`
to be `https`, or plain `http` only on a loopback host - JWKS is the root of
trust for every token the gateway accepts, so this is deliberate, not
something to route around. `deploy/demo/compose.yaml`'s gateway service uses
`network_mode: "service:keycloak"` so the gateway genuinely reaches Keycloak
over loopback (`http://localhost:8080/...`) rather than a bridge-network
hostname - it's the supported loopback shape, not a bypass of the check.
One side effect: the gateway can't publish its own ports, so the root
`compose.yaml` publishes `9090` (the gateway's data plane) from the
`keycloak` service instead.

## Tearing down

```bash
docker compose down -v
# or: podman compose down -v
```

`-v` also drops the `pgdata` volume, so the next `up` starts from the seeded
data again rather than whatever got written during the demo.

## Troubleshooting

- **Gateway keeps restarting / logs "load config" or JWKS errors.** Check
  `docker compose logs keycloak` - the realm import runs once, at Keycloak's
  own startup, and the gateway's entrypoint polls
  `http://localhost:8080/realms/mcplake/protocol/openid-connect/certs`
  until it responds. If Keycloak is healthy but this still fails, the realm
  name or the JWKS path drifted from `deploy/demo/keycloak/mcplake-realm.json`.
- **Token request returns `unauthorized_client` or similar.** Confirm the
  client ID is `mcplake-demo` and that you're posting to the `mcplake`
  realm's token endpoint, not `/realms/master/...`.
- **`docker compose` vs `podman compose` behave differently for
  `network_mode: "service:..."`.** Both support it, but if your Podman
  version doesn't, the gateway container will fail to start with a network
  error - update Podman (`network_mode: service:` needs a reasonably recent
  Podman Compose provider).
