# ADR-0014: OIDC Authorization Code + PKCE Login for the Admin Web UI

- Status: Accepted
- Date: 2026-09-11

## Context

[ADR-0010](0010-control-plane-admin-authentication.md) put every `/admin/*` route
except `GET /admin/healthz` behind a bearer-JWT gate: the token is verified
against the `[oidc]` JWKS (signature, `exp`, `iss`, `aud`) and its claims must
then satisfy the `admin_auth.match` rules. It explicitly deferred one thing:

> The web UI's own static assets are served from the engine root (`/`), not
> `/admin`, so they remain reachable; the UI's *API calls* to `/admin/*` will
> need a token — wiring that into the SPA is tracked under the Admin UI epic
> (#76), not here.

So today an operator who turns on `admin_auth` gets a web UI that loads and then
fails every request with `401`. `CONFIG.md` documents this as a known gap and
tells operators to leave admin auth off behind network isolation — which makes
the gate un-adoptable for anyone who wants the UI.

Requirements for closing it:

- **Reuse the deployment's existing OIDC provider.** The gateway already depends
  on one for data-plane and admin tokens. A second credential system for the UI
  would be the "secret sprawl" ADR-0010 rejected in its Alternative A.
- **No client secret in the browser.** The SPA is a public client; anything it
  ships is readable by anyone who loads the page.
- **One binary, many deployments.** The UI is compiled into the gateway
  (`go:embed webui/dist`) and the same binary runs against different providers,
  so provider coordinates cannot be baked into the bundle at build time.
- **Air-gapped / zero-egress friendly.** The project's whole premise
  ([OVERVIEW.md](../overview.md)) is running with no outbound internet. Anything
  requiring the *gateway* to reach the internet at startup is a non-starter.
- **Backward compatible.** A deployment with `admin_auth` off must see exactly
  today's UI — no login screen, no behaviour change.
- **Distinguish "not signed in" from "not an admin".** `401` and `403` mean
  different things here and have different remedies; conflating them produces a
  login loop that can never succeed.

Non-goals: an identity provider of the gateway's own; per-operation RBAC (still
the single "is an admin" gate ADR-0010 defined); server-side sessions or
cookies; logout at the provider (RP-initiated logout).

## Decision

**1. Authorization Code + PKCE (S256), run entirely in the SPA.**

The UI redirects to the provider's `authorization_endpoint` with
`response_type=code`, a `code_challenge`, and a random `state`; the provider
redirects back to the UI's own origin; the UI verifies `state`, exchanges the
code at the `token_endpoint` with its `code_verifier`, and uses the resulting
access token as the `Authorization: Bearer` on every `/admin/*` call. No client
secret, no server-side callback handler, no new listener. PKCE parameters are
generated with WebCrypto (`crypto.getRandomValues`, `crypto.subtle.digest`) — no
new frontend dependency.

The redirect URI is the UI's own origin + `/`, i.e. the control-plane listener
the operator already browses. The gateway serves the SPA for unmatched paths
already, so no new route is needed to receive the callback.

**2. A public `GET /admin/auth/config` describing how to log in.**

Registered on the `/admin` group *before* the admin-auth middleware, exactly
like `GET /admin/healthz`, so an unauthenticated browser can read it:

```json
{
  "auth_required": true,
  "issuer": "https://auth.example.com",
  "client_id": "mcplake-admin-ui",
  "authorization_endpoint": "https://auth.example.com/authorize",
  "token_endpoint": "https://auth.example.com/oauth/token",
  "scopes": ["openid", "profile", "email"]
}
```

With admin auth off it answers `{"auth_required": false}` and the UI skips the
whole flow. Everything this endpoint returns is public by construction in a PKCE
public client — it is exactly what the SPA would otherwise hard-code and ship in
its bundle. No secret, no JWKS material, no policy content.

**3. Provider coordinates are explicit config, not OIDC discovery.**

A new `[admin_auth.login]` table:

```toml
[admin_auth.login]
client_id = "mcplake-admin-ui"
authorization_endpoint = "https://auth.example.com/authorize"
token_endpoint = "https://auth.example.com/oauth/token"
scopes = ["openid", "profile", "email"]   # optional
```

This mirrors `oidc.jwks_url`, which is already an explicit URL rather than
something discovered from the issuer ("OIDC discovery from a provider/issuer URL
is not yet implemented" — `config.OIDCConfig`). `Config.Validate()` rejects a
partially-filled table at startup.

It lives under `admin_auth` rather than `oidc` because it is meaningless without
the gate: it describes how a *human operating the admin UI* gets a token, not
how tokens are verified. `[oidc]` stays the verification side, shared with the
data plane.

**4. The access token lives in `sessionStorage`; `state`/`code_verifier` too.**

Scoped to one tab and cleared when it closes. Chosen over `localStorage` (shared
across tabs, outlives the browsing session) and over memory-only (a page reload
would bounce the operator through the provider again, which for an operational
console is the difference between usable and not).

**5. `401` and `403` drive different UI states.**

`401` invalidates the session and returns to the login screen (the token is
missing, expired, or rejected by the validator — signing in again can fix it).
`403` shows a distinct "signed in, but these claims aren't an admin" screen
naming the signed-in subject, with sign-out as the only action: `admin_auth.match`
rejected the claims, so re-authenticating as the same principal cannot help.

## Alternatives Considered

### Alternative A: Implicit flow (`response_type=token`)

Advantages:
- Simplest possible browser flow; no token-endpoint call, no PKCE.

Disadvantages:
- The access token lands in the URL fragment — browser history, referrer leakage,
  server logs if the fragment is ever promoted.
- Deprecated: OAuth 2.0 Security BCP (RFC 9700) and OAuth 2.1 both remove it, and
  major providers have disabled it by default.
- No refresh tokens, so the operator is bounced to the provider every time the
  token expires.

### Alternative B: Backend-for-frontend — the gateway runs the flow and sets a session cookie

Advantages:
- The token never reaches JavaScript; immune to token theft via XSS.
- Refresh handled server-side, invisible to the SPA.

Disadvantages:
- Makes the gateway a stateful OIDC *client*: callback route, client secret in
  config, session store (in-memory breaks restarts and any future HA), cookie
  security (`SameSite`, CSRF tokens on every mutating admin call).
- A cookie-authenticated `/admin/*` is ambient authority — CSRF becomes a live
  concern on an API that currently has none because it is bearer-only.
- Two authentication schemes on one surface (bearer for `curl`/MCP clients,
  cookie for the UI), doubling the middleware's behaviour and its test matrix.
- Contradicts ADR-0010's deliberately stateless, config-only design.

### Alternative C: A token-paste form ("paste your JWT here")

Advantages:
- Trivial: no provider coordinates, no config, no redirect, no callback.
- Works for the demo container, which has no real provider at all.

Disadvantages:
- Pushes token acquisition onto every operator: they need their own tooling to
  mint or copy a JWT before the UI is usable.
- Encourages long-lived tokens pasted from a shell — the precise habit a
  redirect flow exists to avoid.
- Not a login *process*, which is what this task is for.

Kept in mind as a possible debug-only escape hatch; deliberately not shipped, so
there is exactly one way in.

### Alternative D: Server-side OIDC discovery (`${issuer}/.well-known/openid-configuration`)

Advantages:
- One config key (`issuer`) instead of three; endpoints can't drift from the
  provider.

Disadvantages:
- Requires the gateway to make an outbound call, at startup or on first request —
  against the zero-egress premise, and a new startup failure mode in exactly the
  deployments that care most.
- Inconsistent with `oidc.jwks_url`, which is already explicit for the same
  reason.
- The demo/test setups use an issuer string that isn't a resolvable URL at all
  (`issuer = "local-demo"`), so discovery would have to be optional anyway.

A discovery *convenience* that fills the three keys when an operator opts in is
plausible future work, not a reason to block the flow on it.

## Decision Criteria

1. No secret material in the browser bundle or in a public endpoint.
2. No outbound network requirement for the gateway itself.
3. No new statefulness in the control plane.
4. Deployments with admin auth off are untouched.
5. Standards-current (OAuth 2.1 / RFC 9700 compliant) rather than merely working.

Authorization Code + PKCE is the only option satisfying all five: A fails 5, B
fails 3 (and weakens 1 by introducing a client secret), C fails the task, D
fails 2.

## Rationale

PKCE exists precisely for public clients that cannot hold a secret, which is
what a `go:embed`-ed SPA is. Keeping the flow in the browser means the gateway's
admin surface stays what ADR-0010 made it — a stateless bearer-token check —
while the UI becomes just another OAuth client of the provider the deployment
already runs.

The public config endpoint is the small amount of dynamism that makes one binary
work against many providers. It is worth being explicit about why it is safe to
leave open: a PKCE client's `client_id` and its provider's endpoints are not
credentials. They are published in the authorization request itself, visible to
anyone who watches the redirect. Exposing them behind the auth gate would be
circular — you would need a token to learn how to get a token.

## Consequences

### Positive

- `admin_auth` becomes adoptable with the web UI, closing ADR-0010's largest
  deferred gap; the `CONFIG.md` caveat telling operators to keep admin auth off
  goes away.
- Admin actions now carry a real, per-person identity (the token's `sub`), which
  is the precondition for the Phase 3 audit trail ADR-0010 also anticipated.
- No new Go dependency, no new frontend dependency, no new listener, no new
  persistence.

### Negative

- Three more config keys for operators who want the UI authenticated, plus a
  client registration at their provider (a public/SPA client with the gateway's
  control-plane URL as an allowed redirect URI).
- The gateway's audience requirement (`oidc.audience`) must be satisfiable by a
  token the provider issues to this client — usually a scope or a provider-side
  audience mapping. The gateway cannot verify that at startup; a misconfiguration
  surfaces as a `401` after an otherwise successful login.
- Token expiry now matters interactively. Mitigated by refreshing when the
  provider issued a `refresh_token`, and by returning the operator to the login
  screen rather than to a broken screen when it did not.

### Risks

- **XSS ⇒ token theft.** An attacker who executes script in the UI's origin can
  read `sessionStorage`. This is inherent to browser-held tokens (Alternative B
  is the only structural fix) and bounded by the token's lifetime and by the
  admin surface being on a trusted interface. The SPA renders no untrusted HTML
  (no `dangerouslySetInnerHTML` anywhere in `webui/src`), which is the primary
  mitigation.
- **Open redirect via the callback.** Mitigated by the redirect URI being the
  UI's own origin, fixed at runtime from `window.location.origin`, never taken
  from a query parameter.
- **Mixing up "not signed in" with "not an admin"** would produce an infinite
  redirect to the provider. Addressed by decision 5 and covered by tests.

### Follow-up

- Optional OIDC discovery to populate `[admin_auth.login]` from the issuer, for
  deployments that do have egress (Alternative D as a convenience).
- RP-initiated logout (`end_session_endpoint`), so signing out of the UI can
  optionally sign the operator out of the provider.
- Attribute admin writes to `claims.Subject` in the audit trail (Phase 3).
- A debug-only token-paste entry point, if operating the demo/air-gapped test
  setups without any provider proves painful (Alternative C).

## Validation

- `config`: `[admin_auth.login]` absent / complete / partially filled (each
  missing key rejected) / `scopes` defaulted.
- `gateway/internal/controlplane`: `GET /admin/auth/config` reachable with the
  admin-auth middleware installed; reports `auth_required: false` with the gate
  off, and the login coordinates with it on; never exposes anything beyond them.
- `webui`: PKCE helper properties (verifier charset/length, S256 challenge is
  base64url of the SHA-256 digest); the session state machine across
  login → callback → authenticated → `401` → login, plus `state` mismatch and
  the `403` terminal state; `client.ts` attaching the header.
- Real-browser end-to-end against the running gateway with admin auth on and a
  live authorization-code redirect, per the webui verification practice
  established in #79/#80/#153.

## References

- [ADR-0010](0010-control-plane-admin-authentication.md) — the gate this login
  feeds; its "teach the web UI to obtain a token (#76)" follow-up is resolved
  here.
- [ADR-0002](0002-jsonpath-regexp-claim-rule-engine.md) — the `admin_auth.match`
  rules that decide `403` vs `200` after a successful login.
- [RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636) — PKCE.
- [RFC 9700](https://datatracker.ietf.org/doc/html/rfc9700) — OAuth 2.0 Security
  Best Current Practice: authorization code + PKCE for browser apps, implicit
  flow removed.
- [CONFIG.md](../../CONFIG.md) — the `admin_auth.login` keys.
- [api/admin.md](../../api/admin.md) — `GET /admin/auth/config`.
