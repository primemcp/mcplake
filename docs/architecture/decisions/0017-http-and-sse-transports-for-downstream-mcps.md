# ADR-0017: HTTP and SSE Transports for Downstream MCPs

- Status: Accepted
- Date: 2026-09-23

## Context

[ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md) established
dynamic registration and schema discovery, and implemented exactly one way to
reach a downstream MCP: spawn it as a subprocess and speak over its stdin/stdout.
`cache.Registry.Register` rejected everything else outright —

```go
if reg.Transport != "stdio" {
    err := fmt.Errorf("cache: unsupported transport %q for %q (only stdio is implemented)", ...)
```

— and `mcp.NewClient` only ever built an `sdk.CommandTransport`.

That was the right starting point, but the consequence is architectural rather
than cosmetic: **every downstream MCP has to be a child process of the gateway.**
It cannot be a separate container, a separate pod, a server on another host, or
a service somebody else operates. Three concrete costs had accumulated:

- The compose demo has to run its MCP server *inside the gateway's own image*,
  which `deploy/demo/Dockerfile`, `compose.yaml` and `docs/DEMO.md` each have a
  paragraph explaining. Adding a second demo MCP means adding its runtime to the
  gateway image too.
- Every registered MCP inherits the gateway's process environment and filesystem.
  That is already tracked as a security concern (#166); it is unavoidable while
  stdio is the only option.
- The admin UI has carried `sse` and `http` as visibly disabled options labelled
  "Not implemented by the gateway yet" since the screen was built, and
  `connectConfigDTO` has carried an unused `url` field. The data model was ready;
  the transport was not.

The vendored `github.com/modelcontextprotocol/go-sdk` v1.7.0 already ships both
client transports (`StreamableClientTransport`, `SSEClientTransport`), so this is
wiring, not protocol work.

## Decision

**Support `stdio`, `http` and `sse` as downstream transports**, selected per
registration, with `stdio` remaining the default for an omitted value.

`mcp.Config` gains `Transport` and `URL`. `mcp.NewClient` switches on the
transport, builds the matching sdk transport, and rejects a config that omits
what its transport needs — `Command` for stdio, `URL` for http and sse. Supplying
the *other* transport's field is also an error rather than being ignored: a `url`
on a stdio entry, or a `command` on an http one, otherwise connects to something
other than what the operator wrote down, silently.

`http` means MCP Streamable HTTP, the current standard. `sse` is the older
HTTP+SSE transport, included because plenty of deployed servers still speak only
that; it is not deprecated *here* any faster than it is upstream.

Transport names are canonical constants owned by `mcp` and re-exported by
`cache`. They are persisted verbatim, accepted verbatim by the admin API, and
written verbatim in `[[mcps]]`'s `type` key, so they are a wire format: renaming
one orphans existing rows.

### The URL carries the same guardrail as the OIDC endpoints

A downstream URL must be **https, or http only on a loopback host** — the same
rule `config.validateSecureHTTPURL` already applies to `oidc.jwks_url` and the
`[admin_auth.login]` endpoints.

The justification is the payload, not the endpoint. A tool call's arguments and
its response are precisely what this gateway exists to control access to: access
policies decide whether the call happens (ADR-0004) and filter policies strip
fields out of the result (ADR-0015). Shipping that same content to a non-loopback
host in the clear would undo one layer while carefully maintaining the other.
Loopback keeps plaintext for the same reason it does elsewhere — local
development, the test fixtures, and a sidecar sharing the gateway's network
namespace all serve over an interface no attacker sits on.

**Enforcement lives in `mcp.NewClient`, not only in `config`.** Config is not the
only write path: `POST /admin/mcps` and the ADR-0011 MCP control server construct
a registration directly and reach `Registry.Register` without passing through
config at all. `config.Validate` additionally checks `[[mcps]]` entries at
startup — which it did not validate at all before this ADR — so an operator gets
an error naming the offending entry rather than a warning buried in the boot log.

To be explicit about what this does *not* change: it is not a new privilege
boundary. An admin who can register a stdio MCP can already start an arbitrary
process on the gateway host, which `docs/CONFIG.md` states plainly. This is a
confidentiality guardrail on a new outbound surface.

### The standalone SSE stream is disabled

`StreamableClientTransport` optionally holds a GET open for the lifetime of the
session so the server can push notifications unprompted. The gateway consumes
none of them: [ADR-0013](0013-periodic-mcp-schema-refresh.md) settled on polling
`tools/list` on an interval, specifically so schema freshness does not depend on
a downstream implementing notifications. Leaving the stream on would hold one
connection open per registered MCP forever, with a reconnect loop behind it, for
no benefit — so `DisableStandaloneSSE` is set.

If the gateway ever starts reacting to server-initiated messages, ADR-0013's
polling is what should be reconsidered, not this flag on its own.

### Timeouts

The shared HTTP client sets **no** `http.Client.Timeout`. An MCP session outlives
any single request, and a whole-request deadline would sever a healthy connection
on a timer. The bounds that matter for a stuck peer are set per phase instead —
dial, TLS handshake, response header — so establishing a connection fails fast
while a stream that has begun is allowed to continue.

## Alternatives Considered

### Alternative A: Keep stdio only; run remote MCPs behind a local proxy

Advantages:
- No change to the gateway at all.
- One transport to reason about, test and secure.

Disadvantages:
- Pushes the problem onto every operator, who must find, run and monitor a
  stdio↔HTTP bridge per remote MCP.
- The bridge becomes an unmonitored component inside the trust boundary, holding
  the same payloads, with none of the gateway's policy machinery.
- Does nothing for the demo, which would still need its MCP inside the gateway
  image.

### Alternative B: Implement Streamable HTTP only, skip SSE

Advantages:
- One fewer transport; SSE is the older mechanism and is being phased out
  upstream.
- Smaller surface to test.

Disadvantages:
- A large installed base of servers still speaks only SSE. Refusing them makes
  the feature unusable for exactly the "a service somebody else operates" case
  that motivates it.
- The sdk already provides the transport; the marginal cost is a case arm and a
  test, not an implementation.

### Alternative C: Allow plaintext http to any host, and warn

Advantages:
- Least friction for an operator with an internal http-only MCP.
- Consistent with "the gateway terminates no TLS anyway".

Disadvantages:
- Inconsistent with the rule the same codebase already applies to `jwks_url` and
  the PKCE token endpoint, for a payload that is at least as sensitive.
- A warning at startup is read once, if ever; the exposure lasts for the
  deployment's lifetime.
- The escape hatches are real and cheap: terminate TLS in front of the MCP, or
  run it as a sidecar sharing the gateway's network namespace, which is what the
  demo does.

### Alternative D: Put the URL check only in `config`

Advantages:
- One place, next to the existing `validateSecureHTTPURL`, no duplication.

Disadvantages:
- Leaves `POST /admin/mcps` and the ADR-0011 control server unguarded — two write
  paths that never touch config. A control present on one of three write paths is
  not a control.

## Decision Criteria

1. Can a downstream MCP run outside the gateway's process? (A: no.)
2. Does it work against servers that exist today, not only new ones? (B: no.)
3. Is the payload protected on the wire by default? (C: no.)
4. Is the guardrail enforced on *every* write path? (D: no.)
5. Does existing stdio behaviour change? (All: no — and it must not.)

Only the chosen design satisfies all five.

## Rationale

The gateway's value is that it sits between a caller and a tool and applies
policy. Requiring the tool to be its own child process is an implementation
detail leaking into the deployment topology, and it was the one thing forcing
the demo's MCP server into the gateway's image.

Adding the transports is small because the sdk already has them. The part worth
deliberating was the URL rule, and the answer follows from consistency: this
codebase already decided that security-relevant material does not travel in
plaintext to a remote host. Tool payloads qualify.

## Consequences

### Positive

- A downstream MCP can be a separate container, pod, or third-party service.
- The demo can run an MCP as its own container instead of a subprocess baked
  into the gateway image.
- A remote MCP does not inherit the gateway's environment or filesystem, which
  narrows #166 for every registration that uses one.
- `[[mcps]]` is validated at startup for the first time: a missing command used
  to surface only as a logged-and-skipped registration failure.
- The test suite gains its first fully successful `POST /v1/call` — 200 with a
  filtered body — because an http downstream is an `httptest.Server` rather than
  a subprocess to manage.

### Negative

- Two more transports to keep working as the sdk evolves.
- An operator with an internal plaintext http MCP on a remote host must front it
  with TLS or colocate it. This is deliberate; see Alternative C.
- A remote MCP introduces network failure modes (DNS, TLS, timeouts) that a
  subprocess did not have. `Register` already records an unreachable MCP rather
  than failing startup, so the shape of the failure is not new, but its causes
  are more varied.

### Risks

- **No outbound authentication.** Nothing here lets the gateway present a bearer
  token or run OAuth against a downstream MCP; `StreamableClientTransport` has an
  `OAuthHandler` slot and it is left nil. A downstream that requires credentials
  cannot be registered yet. This is a known gap, not an oversight — see Follow-up.
- The URL is operator-supplied and the gateway dials it, which is SSRF-shaped.
  It is not an escalation (control-plane access is already host-level access, per
  `docs/CONFIG.md`), but an operator who treats the admin API as less sensitive
  than shell access is mistaken in a new way as well as the existing one.

### Follow-up

- Outbound auth to a downstream MCP (static bearer token, then OAuth via the
  sdk's `OAuthHandler`). This is the main thing standing between "works for a
  sidecar" and "works for a third-party hosted MCP".
- Admin UI support for choosing a transport and entering a URL — the
  `TransportPicker` and `connectConfigDTO.url` are both already in place waiting
  for it.
- Per-MCP timeout/retry configuration, if the shared defaults prove wrong.

## Validation

- `mcp`: a real Streamable HTTP round trip and a real SSE round trip against the
  sdk's own server handler behind `httptest` — connect, `tools/list`,
  `tools/call`; every incomplete-config rejection (missing command, missing URL,
  unknown transport); plaintext-to-a-remote-host refused before anything is
  dialed; the URL rule over a table including `localhost`, `LOCALHOST`,
  `127.0.0.1`, `[::1]`, a non-loopback http host, a non-http scheme and a
  hostname that merely starts with `127.0.0.1`.
- `cache`: http and sse registrations reach `mcp.NewClient` with their transport
  and URL intact and land as `Active` with discovered tools; an unknown transport
  is rejected and still recorded as `Unreachable`; an omitted transport is
  normalized to `stdio` before anything stores it.
- `config`: `validateMCP` over every valid and invalid shape, including the two
  cross-field cases (a url on stdio, a command on http); `Validate` reports a bad
  entry by index *and* name; a config mixing all three transports is accepted.
  A dedicated test runs `mcp.ValidateEndpointURL` and
  `config.validateSecureHTTPURL` over one table and fails if they ever disagree,
  since neither can import the other.
- `cmd/gateway/app`: an http MCP declared in config, end to end through the real
  wiring — discovered, authorized, called, and its `salary_usd` stripped by a
  filter policy, proving policy applies identically regardless of transport; and
  a caller with no grant still gets 403.

## References

- [ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md) — established
  registration and, with it, the stdio-only constraint this lifts.
- [ADR-0013](0013-periodic-mcp-schema-refresh.md) — the polling decision that
  makes the standalone SSE stream unnecessary.
- [ADR-0015](0015-filter-the-tool-payload-not-the-transport-envelope.md) — what
  is in the payload the URL rule protects.
- [CONFIG.md](../../CONFIG.md) — the operator-facing `[[mcps]]` reference.
