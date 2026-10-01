# Pipeline recordings

Source scripts for the gifs/screenshots that will illustrate the gateway's
actual request pipeline (auth → router → cache → filter, plus the control
plane's own write path) for the README and the upcoming GitHub Pages landing
page. This directory holds the **recording pipelines** (scripts that
produce the media) — running them is a separate, later step from writing
them.

Every scenario runs against the real [compose demo](../../getting-started/demo.rst), using
its real entities — `alice`/`bob`/`mcplake-admin`, `employee-directory`/
`inventory`, `hide-sensitive-fields-get-employee` — not placeholder
names, so a viewer who then reads that walkthrough recognizes exactly what
they just watched.

## Scenarios

| # | File | Pipeline stage | What it shows | Status |
|---|------|-----------------|----------------|--------|
| 01 | `tapes/01-auth-three-tokens.tape` | Auth | Three users, one Keycloak realm, one claim (`role`) that drives everything downstream. | ✅ recorded |
| 02 | `tapes/02-filter-alice-vs-admin-get-employee.tape` | Filter (ADR-0015) | Same tool, same employee, called as alice then mcplake-admin — one response has `salary_usd`/`ssn_last4` stripped, the other doesn't. | ✅ recorded |
| 03 | `tapes/03-router-bob-forbidden.tape` | Router — deny path | A valid token that matches no `access_policy` never reaches the MCP. | ✅ recorded |
| 04 | `tapes/04-router-cross-mcp-grants.tape` | Router — per-MCP grants | alice's grant names one MCP; mcplake-admin's wildcard reaches both, stdio and sse alike (ADR-0017). Targets `inventory`, not `demo-postgres` — see below. | ✅ recorded |
| 05 | `tapes/05-enabled-gate-toggle.tape` | Enabled gate | Toggling `employee-directory` off via `PATCH /admin/mcps/:name` turns a live 200 into `403 mcp_disabled`, reversibly. | ✅ recorded |
| 06 | `browser/06-admin-oidc-signin.mjs` | Auth, from the UI | Real Authorization Code + PKCE round trip through Keycloak (ADR-0014), not a ROPC curl. | ✅ recorded |
| 07 | `browser/07-request-path-simulation-alice.mjs` | The whole pipeline, visualized | Users & access → demo-reader → **Request path** tab — the UI's own Client/Agent → Auth Validator → Access Check → Response Filter nodes. | ✅ recorded |
| 08 | `browser/08-inspector-register-mcp.mjs` | Control plane write path | MCP Inspector driving `/admin/mcp` itself (ADR-0011). | ⛔ blocked — see below |

## Known issues hit while recording (2026-09-28)

Real, reproducible problems found running the actual compose demo — worth
fixing separately from this directory's own purpose:

- ~~**Keycloak 26.0 rejects every demo login with "Account is not fully
  set up".**~~ **Fixed.** The realm's default `VERIFY_PROFILE` required
  action (on by default since Keycloak ~24, not something
  `mcplake-realm.json` had ever declared) fired because the demo users'
  `attributes.role` custom claim isn't declared in the realm's User
  Profile schema. During this recording session it was worked around live
  via `kcadm.sh update authentication/required-actions/VERIFY_PROFILE -r
  mcplake -s enabled=false`; that isn't persisted, so a fresh
  `docker compose up` hit it again every time. Now fixed for real:
  `mcplake-realm.json` declares `VERIFY_PROFILE` disabled in its own
  `requiredActions` export, so a clean import never hits it — verified
  against a real `docker compose up keycloak` with no manual patching.
- ~~**`demo-postgres` never registers.**~~ **Worked around**, not fixed —
  this one isn't ours to fix. The gateway's SSE client always sends a
  `server/discover` probe (SEP-2575) before `initialize`; `postgres-mcp`'s
  Python `mcp` SDK doesn't recognize it and tears down the SSE session in
  response (visible in its logs as a pydantic `ValidationError`) instead of
  answering with a JSON-RPC "method not found", so the client's *correct*
  per-spec fallback to legacy `initialize` fails too, on the same dead
  connection. `crystaldba/postgres-mcp` hasn't published a release since
  2025-05-16 (`0.3.0` is still latest), predating SEP-2575 entirely, so
  there's no newer image to move to, and this isn't a bug in our own
  `go-sdk` usage to patch. Scenario 04 now targets `inventory` instead — a
  FastMCP server the demo already owns, also over `sse`, in its own
  container (`inventory-mcp`), unaffected because it's built on a current
  MCP SDK. `demo-postgres` itself is untouched and still documents the
  "a real third-party server, not one we wrote" story elsewhere in the demo
  — this only affects which MCP the recording pipeline calls.
- **MCP Inspector 2.7.0 has no manual-header field for `streamable-http`
  servers.** `docs/DEMO.md`'s Inspector section says to paste
  `Authorization: Bearer <token>` as a header, but the pinned image's
  "Add server" / "Edit server" / "Server Settings" modals only offer
  Server ID / Transport / URL (plus protocol-era options) — auth is
  OAuth/Enterprise-IdP-only in this version. Either `docs/DEMO.md` is
  describing an older Inspector UI, or the compose pin needs revisiting.
  Blocks scenario 08.

## Prerequisites (for the capture step, not for reading this)

```bash
brew install vhs          # terminal recordings -> gif (pulls in ttyd, ffmpeg)
cd browser && npm install && npx playwright install chromium   # browser recordings
```

`browser/package.json` is a throwaway dependency manifest for these
scripts only — not part of the shipped webui module. `node_modules` isn't
committed; re-run `npm install` before recording again.

Bring up the real demo stack first (not `mcplake-dev`/`mcplake-demo`, the
compose stack — see [docs/DEMO.md](../../DEMO.md#running-it)):

```bash
docker compose up -d --build
```

## Running a terminal scenario

```bash
cd docs/media/pipeline/tapes
vhs 02-filter-alice-vs-admin-get-employee.tape
```

Each `.tape` sources `lib/env.sh` for token-fetching/call helpers, so the
on-screen commands stay about the pipeline (`call_gateway "$ALICE" ...`),
not about curl/jq plumbing. Output lands in `../out/`.

## Running a browser scenario

```bash
cd docs/media/pipeline/browser
node 06-admin-oidc-signin.mjs
```

These record a `.webm` (via Playwright's `recordVideo`) and one payoff
`.png`. Convert the video to a gif:

```bash
ffmpeg -i ../out/<generated>.webm -vf "fps=12,scale=1280:-1" ../out/06-admin-oidc-signin.gif
```

## Status

01 through 07 are recorded, converted to gif, and verified frame-by-frame
against the real running compose demo — `out/` has the result. 08 is
blocked on the real, reproducible Inspector issue above, not on the
recording script itself.
