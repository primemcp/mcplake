# Demo: Compose Stack

A turnkey stack that brings up the gateway end to end - Keycloak as the OIDC
provider, two MCP servers, Postgres, and MCP Inspector - with one command
and no manual setup. See [`compose.yaml`](../compose.yaml) for the services and
[`config.example.toml`](../config.example.toml)/[`CONFIG.md`](CONFIG.md) for
what a real deployment's config file looks like; this demo's own config is
[`deploy/demo/config.toml`](../deploy/demo/config.toml).

## What it stands up

| Service        | Role                                                                                 |
|----------------|--------------------------------------------------------------------------------------|
| `keycloak`     | OIDC provider. A realm (`mcplake`) is pre-imported with a client and three demo users. |
| `gateway`      | The mcplake gateway, built from this repo: the data plane fronting both MCPs below, and the control plane serving the embedded admin web UI. |
| `postgres-mcp` | An MCP server in its **own container**, reached over the `sse` transport. |
| `postgres`     | Backs the gateway's own `[persistence]` (a `gateway` database) *and* the `demo` database `postgres-mcp` reads. The two are deliberately separate. |
| `inspector`    | [MCP Inspector](https://modelcontextprotocol.io/docs/2026-07-28/tools/inspector), for driving the gateway's own MCP control server from a browser. |

### The two MCPs, and why there are two

**`employee-directory`** is `deploy/demo/mcp-servers/employee_directory.py`, a
small [FastMCP](https://gofastmcp.com) server over an in-memory, fictional
employee directory. It runs as a subprocess inside the gateway's own
container, because it is a **stdio** MCP and stdio means a subprocess. Three
tools:

- `list_employees` / `get_employee` return structured records with two
  sensitive fields (`salary_usd`, `ssn_last4`) - see **Field-level
  filtering** below.
- `list_departments` returns plain department names - nothing sensitive,
  nothing to filter, included on purpose for contrast: not every tool needs
  a filter_policy.

**`demo-postgres`** is [crystaldba/postgres-mcp](https://github.com/crystaldba/postgres-mcp),
a real MCP server with its own image, its own container and its own
lifecycle, reached over the **`sse`** transport at
`http://localhost:8000/sse`. It exposes SQL tools (`list_schemas`,
`list_objects`, `get_object_details`, `execute_sql`, `explain_query`, …)
over a small fictional customers/tickets database.

Until [ADR-0017](architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps.md)
this second shape was impossible: stdio was the only transport, so *every*
downstream MCP had to be a child process of the gateway. It now isn't, and
having one of each is the clearest way to show that the gateway's behaviour -
authorization, filtering, the admin API, the UI - does not vary by transport.

> **Why is the container's URL `localhost`?** `mcp.ValidateEndpointURL`
> requires `https`, or `http` only on a loopback host, because a tool call's
> arguments and response are exactly what `access_policies` and
> `filter_policies` exist to control. `compose.yaml` puts `postgres-mcp` in
> the same network namespace as the gateway, so `localhost:8000` genuinely
> is loopback - the supported shape, not a waiver. It is still a separate
> container; only the network namespace is shared. Point the gateway at a
> service hostname over plaintext instead and it refuses to start, on
> purpose.

> **`demo-postgres` has no `filter_policies`, and that is the interesting
> part.** `filter_policies` strip fields out of *structured* JSON
> ([ADR-0015](architecture/decisions/0015-filter-the-tool-payload-not-the-transport-envelope.md));
> a SQL tool that returns a rendered result set has no stable field to
> address, so there is nothing for a `drop_fields` path to match. The lever
> that does work on this MCP is **access control** - who may call it at all,
> and which tools. Both levers, side by side, on two MCPs: that is the
> point of the pairing.
>
> (This is also why an earlier revision of the demo *replaced* its postgres
> MCP with the FastMCP one: as the only MCP it could not demonstrate
> filtering. As the second of two, it demonstrates something else.)

Three demo users exercise claims-based access control (ADR-0002/ADR-0004)
end to end:

- **alice** (`role=db-reader`) is granted `employee-directory` — and *only*
  `employee-directory` — by the `demo-reader` access policy → her calls to
  it succeed, with `salary_usd` and `ssn_last4` stripped by
  `filter_policies`. Her calls to `demo-postgres` get `403 forbidden`: the
  grant names one MCP, so having a token is not having access to everything
  behind the gateway.
- **bob** (`role=guest`) authenticates fine but matches no access policy →
  his calls get `403 forbidden` on both MCPs.
- **mcplake-admin** (`role=admin`) is granted access to every MCP by the
  `admin` access policy — including `demo-postgres`, which nobody else can
  reach — and calls them *unfiltered* - both `filter_policies`
  entries below only match `role=db-reader`, so `role=admin` never triggers
  them. Also the only user `admin_auth.match` lets into `/admin/*` and the
  admin web UI (ADR-0010/ADR-0014) → alice and bob can both get a token
  from Keycloak, but only mcplake-admin can sign into the
  UI.

  This is a `mcplake` realm user, deliberately named to be unmistakable from
  Keycloak's *own* bootstrap superuser (`KEYCLOAK_ADMIN=admin`, in the
  `master` realm - that one logs into Keycloak's own admin console at
  http://localhost:8080/admin, not the gateway).

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
realm and Postgres to actually be ready, and checks that the demo MCP server
can actually run, before starting - so a clean exit with no gateway errors
means the stack is up.

Leave it running in that terminal, or add `-d` to run detached.

### Upgrading or resetting the demo

**After pulling a new revision of this repo, use both `--build` and `-v`:**

```bash
docker compose down -v && docker compose up -d --build
# or: podman compose down -v && podman compose up -d --build
```

Neither flag is optional, and each covers a different half of the stack:

- **`--build` (the image).** `deploy/demo/config.toml` and
  `deploy/demo/mcp-servers/` are bind mounts, so a `git pull` changes them
  immediately. The Python interpreter and `fastmcp` are baked into the image
  and only change on a rebuild. Skip `--build` after pulling a revision that
  changed the demo MCP's dependencies and the two halves disagree.
  `entrypoint.sh` checks for exactly this and refuses to start, naming the
  fix, rather than letting the gateway come up with an MCP it can never
  reach.
- **`-v` (the volume).** Two reasons, both real.

  Config seeding happens **once per named entry, ever**
  ([ADR-0016](architecture/decisions/0016-config-seeding-happens-once-per-entry.md)):
  the database is the source of truth, and `config.toml` only ever seeds a
  name the store has not seen. So an entry that changed *without* changing
  its name is ignored on an upgrade. When the demo replaced its
  `postgres-demo` MCP with `employee-directory`, the `demo-reader` access
  policy kept its own name - so on a surviving volume it goes on granting
  `postgres-demo`, alice gets `403` on the MCP that actually exists, and the
  dead `postgres-demo` registration sits in the admin UI as permanently
  unreachable.

  And Postgres only runs `docker-entrypoint-initdb.d` on an **empty data
  directory**. The `demo` database that `postgres-mcp` reads is created
  there (`deploy/demo/postgres/02-demo-data.sql`), so on a volume from
  before that file existed it is simply absent, and `postgres-mcp` fails to
  connect.

  Dropping the volume is the right fix for a demo. In a real deployment you
  would make the change through the admin API instead, which is the whole
  point of the rule. Either way the gateway now logs a `WARN` naming any
  entry whose stored record disagrees with `config.toml`, so this is visible
  rather than silent - see [CONFIG.md](CONFIG.md#3-persistence).

Between pulls, `up -d` on its own is fine: the config and the MCP script are
mounts, so a `compose restart gateway` picks up edits to either without a
rebuild.

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
  -d '{"mcp":"employee-directory","tool":"get_employee","arguments":{"employee_id":2}}' | jq
```

This returns `200` with Marcus Webb's record — **minus `salary_usd` and
`ssn_last4`**, stripped by the `hide-sensitive-fields-get-employee` filter
policy. See **Field-level filtering** below for the side-by-side against
mcplake-admin, who sees them.

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
  -d '{"mcp":"employee-directory","tool":"get_employee","arguments":{"employee_id":2}}'
```

This prints `403` - bob authenticated (Keycloak issued him a token) but no
`[[access_policies]]` entry grants his `role=guest` claim access to
`employee-directory`.

`GET /healthz` on the data plane needs no token:

```bash
curl -s http://localhost:9090/healthz
```

### The MCP that isn't in the gateway's container

Everything above went through `employee-directory`, a subprocess of the
gateway. `demo-postgres` is a different container entirely, and calling it
looks identical from the outside — which is the point:

```bash
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8080/realms/mcplake/protocol/openid-connect/token \
  -d grant_type=password \
  -d client_id=mcplake-demo \
  -d username=mcplake-admin \
  -d password=mcplake-admin \
  -d scope=openid | jq -r .access_token)

curl -s -X POST http://localhost:9090/v1/call \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mcp":"demo-postgres","tool":"execute_sql","arguments":{"sql":"SELECT company, region, plan, arr_usd FROM customers ORDER BY arr_usd DESC"}}' | jq
```

Same endpoint, same token format, same pipeline. The gateway reached out over
`sse` to another container instead of writing to a pipe, and nothing above
the transport had to know.

Now the same call as **alice**, who is granted `employee-directory` and
nothing else:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST http://localhost:9090/v1/call \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mcp":"demo-postgres","tool":"execute_sql","arguments":{"sql":"SELECT 1"}}'
```

`403`. Alice has a perfectly valid token and an access policy that works —
for the other MCP. This is the per-MCP grant in `[[access_policies]]` doing
the one thing a single-MCP demo can never show.

Worth trying too: `list_schemas` and `list_objects` to see what the MCP
advertises about the database, and `explain_query` on the SELECT above.

## Field-level filtering

Two `[[filter_policies]]` entries in `deploy/demo/config.toml` strip
`salary_usd`/`ssn_last4` from `employee-directory`'s two tools whenever the
caller's `role` is `db-reader` — alice's calls above already show this, but
seeing it side-by-side against an unfiltered call makes the point clearer.
Get a token for mcplake-admin the same way as before:

```bash
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8080/realms/mcplake/protocol/openid-connect/token \
  -d grant_type=password \
  -d client_id=mcplake-demo \
  -d username=mcplake-admin \
  -d password=mcplake-admin \
  -d scope=openid | jq -r .access_token)

curl -s -X POST http://localhost:9090/v1/call \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mcp":"employee-directory","tool":"get_employee","arguments":{"employee_id":2}}' | jq
```

Same tool, same employee, `200` both times — but mcplake-admin's response
carries `salary_usd` and `ssn_last4`, alice's doesn't. Neither
`filter_policies.match` rule matches `role=admin`, so admin's call is never
touched.

`list_employees` strips the same two fields from every row
(`$.employees[*].salary_usd`), worth trying too:

```bash
curl -s -X POST http://localhost:9090/v1/call \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mcp":"employee-directory","tool":"list_employees","arguments":{}}' | jq
```

`list_departments` has no filter_policies entry at all — nothing sensitive
in its response, so there's nothing to strip:

```bash
curl -s -X POST http://localhost:9090/v1/call \
  -H "Authorization: Bearer $ALICE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"mcp":"employee-directory","tool":"list_departments","arguments":{}}' | jq
```

**Why `list_employees`'s filter path is `$.employees[*].salary_usd`, not
`$.result[*].salary_usd`:** FastMCP wraps a bare `list[Model]` return as
`{"result": [...]}` in `structuredContent`, but leaves the human-readable
`content[].text` copy of the same result as a bare array — two different
shapes for one tool. A path written against the wrapped shape would
silently miss the unwrapped copy and leak the field through it (`filter.Strip`
treats a path matching nothing as a no-op, not an error — there's no crash
to notice). `employee_directory.py`'s `list_employees` returns an explicit
`EmployeeList { employees: [...] }` model instead of a bare list specifically
to avoid this — both copies end up identically shaped, so one path reaches
both. This was caught by literally running the real filter package against
real FastMCP output while building this demo, not assumed from the docs —
worth knowing if you add your own list-returning tool with a filter on it.

## Admin web UI

The gateway serves its embedded admin UI from the same listener as
`/admin/*` (`control_plane_addr`), so once the stack is up it's just a
browser away at **http://localhost:9091/**.

`deploy/demo/config.toml`'s `[admin_auth]` section gates it: only a token
whose `role` claim is `admin` gets past `/admin/*` (ADR-0010), and
`[admin_auth.login]` tells the UI itself how to run the Authorization
Code + PKCE flow against Keycloak (ADR-0014) - no manual token-pasting.

1. Open http://localhost:9091/ - the UI redirects to Keycloak.
2. Sign in as **mcplake-admin** / **mcplake-admin** (not Keycloak's own
   `admin`/`admin` console login - see the note above).
3. Keycloak redirects back to the UI, now authenticated - try alice/alice or
   bob/bob instead and you'll land back on the UI signed in, but on the
   "signed in, but these claims aren't an admin" `403` screen ADR-0014
   describes, since neither has `role=admin`.

Unlike alice/bob (used above for the ROPC / `curl` flow, since they don't
need a browser), this is the one flow in the demo that exercises Keycloak's
real redirect-based login rather than a password grant - a closer match to
how a human operator would actually sign in.

Two things about this setup that are demo shortcuts, not what a real
deployment should copy verbatim:

- `control_plane_addr = ":9091"` binds every interface, not just loopback.
  That's only safe here *because* `admin_auth` is on - see the comment in
  `deploy/demo/config.toml` and
  [ADR-0005](architecture/decisions/0005-use-gin-for-control-plane-api.md)
  for why the project's own default is loopback-only until it is.
- The Keycloak client's `redirectUris`/`webOrigins` are `["*"]` -
  convenient for a demo that doesn't know its own host/port in advance, but
  a real client registration should list the exact control-plane URL(s)
  operators browse to.

## MCP Inspector

[MCP Inspector](https://modelcontextprotocol.io/docs/2026-07-28/tools/inspector)
is the reference MCP client. The demo runs it so you can drive an MCP server
from a browser without installing anything:

    http://localhost:6274/

The session token is pinned to `mcplake-demo-inspector` (see the security
note below), so you can paste it when Inspector asks rather than digging it
out of the logs.

### What to point it at

**The gateway's own MCP control server.** `deploy/demo/config.toml` enables
`[admin_mcp]`, which mounts the control-plane admin operations as MCP tools
at `/admin/mcp`
([ADR-0011](architecture/decisions/0011-mcp-control-server.md)). In
Inspector:

- Transport: **Streamable HTTP**
- URL: `http://localhost:9091/admin/mcp`
- Header: `Authorization: Bearer <mcplake-admin's token>` — the same
  `$ADMIN_TOKEN` from the sections above. `/admin/mcp` sits behind the
  `admin_auth` gate, so alice's and bob's tokens get `403` here exactly as
  they do against the REST API.

You can then register an MCP, toggle one off, or edit a policy *through MCP
itself*, and watch the change show up in the admin web UI on the next
refresh. That is ADR-0011's whole premise, and until now the demo had no way
to try it.

**Either downstream MCP, directly.** `http://localhost:8000/sse` (transport:
SSE) reaches `postgres-mcp` without going through the gateway at all.

### What Inspector cannot do here

**You cannot point Inspector at the gateway's data plane.** `POST /v1/call`
is a bespoke REST endpoint, not an MCP endpoint — the gateway does not speak
MCP to its callers, it speaks MCP to its *downstreams*. There is nothing on
`:9090` for an MCP client to connect to.

The consequence matters: pointed at a downstream directly, Inspector sees the
**unfiltered** response. Call `get_employee` through `postgres-mcp`'s
neighbour and you would see `salary_usd` — not because filtering is broken,
but because you went around the thing that does the filtering. Read it as the
control case for the side-by-side in **Field-level filtering** above: the raw
server's output, next to what alice actually receives.

### Security, deliberately

Inspector's backend spawns processes on request, and an unauthenticated one
has been a critical RCE before
([CVE-2025-49596](https://nvd.nist.gov/vuln/detail/CVE-2025-49596)). So, in
this compose file:

- Its port is published on **`127.0.0.1` only**, not every interface like
  Keycloak's and the gateway's are.
- Authentication is **left on**. `DANGEROUSLY_OMIT_AUTH` is not set and
  should not be.
- The image tag is pinned (`2.7.0`) rather than tracking `latest`.

The pinned session token is a convenience that is *only* defensible because
of the loopback binding — it is committed to a public repository, so anyone
who can reach that port already knows it. If you publish this port more
widely, drop `MCP_INSPECTOR_API_TOKEN` and read the generated token from
`docker compose logs inspector` instead, and set `ALLOWED_ORIGINS` to
whatever origin you browse from.

**None of this belongs in a production deployment.** Inspector is developer
tooling; so, for that matter, is `[admin_mcp]` being enabled with the
control plane bound to all interfaces, which this demo does for convenience
and `README.md` warns about.

## What's intentionally not in this demo

- **No writable tool.** Every `employee_directory.py` tool is read-only, and
  `postgres-mcp` runs with `--access-mode=restricted` (read-only
  transactions) against a role granted only `SELECT`. Kept simple on
  purpose, not a limitation of the mechanism - a real MCP can freely expose
  a mutating tool, and `access_policies`/`filter_policies` apply to it
  exactly the same way.
- **No TLS anywhere.** Every URL here is plaintext http on loopback, which
  is the one case `mcp.ValidateEndpointURL` and
  `config.validateSecureHTTPURL` permit. A real deployment reaching a real
  remote MCP needs https; see
  [CONFIG.md](CONFIG.md#4-mcp-servers).
- **No outbound authentication to a downstream MCP.** The gateway cannot yet
  present a bearer token or run OAuth against an MCP it connects to, so
  `demo-postgres` is open to anything that can reach its port - which is why
  that port is loopback-only. Tracked in ADR-0017's follow-ups.

## Why four services share one network namespace

`gateway`, `postgres-mcp` and `inspector` all carry
`network_mode: "service:keycloak"`, so the four share one network stack and
reach each other over `localhost`. This is doing three jobs at once, all the
same job really:

1. **`oidc.jwks_url`.** `config.validateSecureHTTPURL` (`config/config.go`)
   requires it to be `https`, or plain `http` only on a loopback host - JWKS
   is the root of trust for every token the gateway accepts, so this is
   deliberate, not something to route around. Sharing the namespace means
   the gateway genuinely reaches Keycloak over loopback
   (`http://localhost:8080/...`) rather than a bridge-network hostname.
2. **`demo-postgres`'s URL.** `mcp.ValidateEndpointURL` applies the same rule
   to a downstream MCP endpoint (ADR-0017), for the payload's sake rather
   than the key's. `http://localhost:8000/sse` satisfies it honestly.
3. **Inspector.** It needs to reach `localhost:9091/admin/mcp` and
   `localhost:8000/sse` from inside its own container.

In every case it is the *supported loopback shape*, not a bypass of the
check. A real deployment reaching a real remote MCP uses https; the demo
colocates instead, which is the other documented option.

One side effect: only the namespace's owner can publish ports, so
`compose.yaml` publishes `9090` (data plane), `9091` (control plane / admin
UI), `8000` (postgres-mcp) and `6274` (Inspector) from the `keycloak`
service. The last two are bound to `127.0.0.1`; see the Inspector section on
why.

## Tearing down

```bash
docker compose down -v
# or: podman compose down -v
```

`-v` also drops the `pgdata` volume, so the next `up` starts from an empty
`gateway` database again rather than whatever the control plane wrote
during the demo (registrations, policies), and re-runs the init scripts that
create the `demo` database `postgres-mcp` reads. `employee-directory`'s own
data is in-memory (`employee_directory.py`) and always resets with the
container regardless.

## Troubleshooting

- **`demo-postgres` is unreachable, or the gateway waits forever on
  "waiting for the Postgres MCP".** Look at that container first:

  ```bash
  docker compose logs postgres-mcp
  ```

  The usual cause is the `demo` database not existing — Postgres runs its
  init scripts only on an empty data directory, so a volume that predates
  `deploy/demo/postgres/02-demo-data.sql` has no `demo` database and no
  `demo_reader` role. See
  [Upgrading or resetting the demo](#upgrading-or-resetting-the-demo);
  `down -v` fixes it.
- **The gateway refuses to start with `config: mcps[...]: url: must use
  https`.** Something changed `demo-postgres`'s URL away from a loopback
  host. That check is deliberate
  ([ADR-0017](architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps.md)):
  the demo gets to use plaintext only because `compose.yaml` puts that
  container in the gateway's own network namespace, which makes
  `localhost:8000` genuinely loopback. Either keep it loopback or put TLS in
  front of the MCP.
- **Inspector says `403` connecting to `/admin/mcp`.** It is behind
  `admin_auth`, so it needs `mcplake-admin`'s bearer token in an
  `Authorization` header — alice's and bob's tokens authenticate but are not
  admins. See the [MCP Inspector](#mcp-inspector) section.
- **Inspector's UI loads but connects fail with a `403` about origins.** You
  remapped its published port. Inspector validates the browser's `Origin`
  against its own bind port, so either publish it as `6274:6274` as
  `compose.yaml` does, or set `ALLOWED_ORIGINS` to the origin you browse
  from (it *replaces* the default list rather than merging with it).
- **The admin UI lists `employee-directory` but it is never connected, shows
  zero tools, and nothing you do to it helps.** That is a registration
  failure: `cache.Registry.Register` records an MCP it cannot reach as
  `unreachable` and the gateway serves on without it (ADR-0003). With zero
  discovered tools there is also nothing for the response-filter editor to
  offer, and saving an edit just re-runs the same failing exec - hence the
  endpoint feeling inert. Check why:

  ```bash
  docker compose logs gateway | grep -i "registration failed"
  ```

  On a current image `entrypoint.sh` catches the common cause before the
  gateway starts, so if you are seeing this at all, the stack is probably
  running an image from before that check existed - see
  [Upgrading or resetting the demo](#upgrading-or-resetting-the-demo).
- **`entrypoint: python3 is not installed in this image` (or `fastmcp is not
  importable`), and the gateway exits.** Working as intended: the image
  predates the demo's Python MCP server while the bind-mounted `config.toml`
  already references it. Rebuild - `docker compose up -d --build`.
- **alice gets `403 forbidden` on a tool that clearly exists.** Check whether
  the gateway logged `config entry already seeded and DIFFERS from the stored
  record` at startup. On a volume that survived an upgrade, the stored
  `demo-reader` policy still grants whatever MCP it was first seeded with -
  see [Upgrading or resetting the demo](#upgrading-or-resetting-the-demo).
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
