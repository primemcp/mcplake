# Pipeline recordings

Source scripts for the gifs/screenshots that illustrate the gateway's
actual request pipeline (auth → router → cache → filter, plus the control
plane's own write path) for the README and the GitHub Pages landing page.
This directory holds the **recording pipelines** (scripts that produce the
media), and the media itself under `out/`.

Every scenario runs against the real [compose demo](../../getting-started/demo.rst),
using its real entities — `alice`/`bob`/`mcplake-admin`, `employee-directory`/
`inventory`, `hide-sensitive-fields-get-employee` — not placeholder names,
so a viewer who then reads that walkthrough recognizes exactly what they
just watched.

## Scenarios

| # | File | Pipeline stage | What it shows |
|---|------|-----------------|----------------|
| 01 | `tapes/01-auth-three-tokens.tape` | Auth | Three users, one Keycloak realm, one claim (`role`) that drives everything downstream. |
| 02 | `tapes/02-filter-alice-vs-admin-get-employee.tape` | Filter (ADR-0015) | Same tool, same employee, called as alice then mcplake-admin — one response has `salary_usd`/`ssn_last4` stripped, the other doesn't. |
| 03 | `tapes/03-router-bob-forbidden.tape` | Router — deny path | A valid token that matches no `access_policy` never reaches the MCP. |
| 04 | `tapes/04-router-cross-mcp-grants.tape` | Router — per-MCP grants | alice's grant names one MCP; mcplake-admin's wildcard reaches both, stdio and sse alike (ADR-0017). Targets `inventory`, not `demo-postgres` — see below. |
| 05 | `tapes/05-enabled-gate-toggle.tape` | Enabled gate | Toggling `employee-directory` off via `PATCH /admin/mcps/:name` turns a live 200 into `403 mcp_disabled`, reversibly. |
| 06 | `browser/06-admin-oidc-signin.mjs` | Auth, from the UI | Real Authorization Code + PKCE round trip through Keycloak (ADR-0014), not a ROPC curl. |
| 07 | `browser/07-request-path-simulation-alice.mjs` | The whole pipeline, visualized | Users & access → demo-reader → **Request path** tab — the UI's own Client/Agent → Auth Validator → Access Check → Response Filter nodes. |
| 08 | `browser/08-inspector-register-mcp.mjs` | Control plane write path | MCP Inspector driving `/admin/mcp` itself (ADR-0011) via its real `Custom Headers` panel (Inspector ≥2.9.0) — `list_mcps` over real MCP, not REST. |

All eight are recorded, converted to gif, and verified frame-by-frame
against the real running compose demo — `out/` has the result.

## Issues hit while recording, and how each was resolved

Real, reproducible problems found running the actual compose demo. Two
were genuinely ours to fix; one wasn't.

- **Fixed — Keycloak 26.0 rejected every demo login with "Account is not
  fully set up".** The realm's default `VERIFY_PROFILE` required action
  (on by default since Keycloak ~24, not something `mcplake-realm.json`
  had ever declared) fired because the demo users' `attributes.role`
  custom claim isn't declared in the realm's User Profile schema. Fixed by
  declaring `VERIFY_PROFILE` disabled in `mcplake-realm.json`'s own
  `requiredActions` export, so a clean import never hits it — verified
  against a real `docker compose up keycloak` with no manual `kcadm.sh`
  patching.
- **Worked around, not fixed — `demo-postgres` never registers.** This one
  isn't ours to fix. The gateway's SSE client always sends a
  `server/discover` probe (SEP-2575) before `initialize`; `postgres-mcp`'s
  Python `mcp` SDK doesn't recognize it and tears down the SSE session in
  response (visible in its logs as a pydantic `ValidationError`) instead of
  answering with a JSON-RPC "method not found", so the client's *correct*
  per-spec fallback to legacy `initialize` fails too, on the same dead
  connection. `crystaldba/postgres-mcp` hasn't published a release since
  2025-05-16 (`0.3.0` is still latest), predating SEP-2575 entirely, so
  there's no newer image to move to, and this isn't a bug in our own
  `go-sdk` usage to patch. Scenario 04 targets `inventory` instead — a
  FastMCP server the demo already owns, also over `sse`, in its own
  container (`inventory-mcp`), unaffected because it's built on a current
  MCP SDK. `demo-postgres` itself is untouched and still demonstrates the
  "a real third-party server, not one we wrote" story elsewhere in the
  demo — this only affects which MCP the recording pipeline calls.
- **Fixed — MCP Inspector 2.7.0 had no manual-header field for
  `streamable-http` servers.** Auth was OAuth/Enterprise-IdP-only; there
  was no way to paste a bearer token for `/admin/mcp` at all. Inspector
  2.9.0 adds a real `Custom Headers` panel (per-server `Settings` →
  `Custom Headers` → `+ Add Header`). Fixed by bumping `compose.yaml`'s
  pinned tag from `2.7.0` to `2.9.0` and updating
  `docs/getting-started/demo.rst`'s Inspector walkthrough to describe the
  actual UI path — verified live: added the server, set
  `Authorization: Bearer <mcplake-admin's token>` under Custom Headers,
  connected, and ran `list_mcps` for a real result.

## Prerequisites (for the capture step, not for reading this)

```bash
brew install vhs          # terminal recordings -> gif (pulls in ttyd, ffmpeg)
cd browser && npm install && npx playwright install chromium   # browser recordings
```

`browser/package.json` is a throwaway dependency manifest for these
scripts only — not part of the shipped webui module. `node_modules` isn't
committed; re-run `npm install` before recording again.

Bring up the real demo stack first (not `mcplake-dev`/`mcplake-demo`, the
compose stack — see [the demo walkthrough](../../getting-started/demo.rst)):

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

`08-inspector-register-mcp.mjs` needs `MCPLAKE_ADMIN_TOKEN` set first —
see the comment at its top.

These record a `.webm` (via Playwright's `recordVideo`) and one payoff
`.png`. Convert the video to a gif:

```bash
ffmpeg -i ../out/<generated>.webm -vf "fps=12,scale=1280:-1" ../out/06-admin-oidc-signin.gif
```
