# ADR-0020: Serve MCP on the Data Plane (Streamable HTTP and SSE)

- Status: Accepted
- Date: 2026-09-23

## Context

[ADR-0001](0001-use-fasthttp-for-gateway-server.md) built the data plane as a
fasthttp server with a hand-written route table, and it has carried exactly two
routes since: `GET /healthz` and `POST /v1/call`. The second is a proprietary
shape invented here —

```json
{ "mcp": "postgres-ro", "tool": "get_user", "arguments": { "id": 42 } }
```

— and it is the only way in. The consequence is blunt: **no MCP client can
connect to this gateway.** Not Claude Code, not an MCP-enabled editor, not MCP
Inspector. A product whose entire purpose is to sit in front of MCP servers does
not speak MCP on the surface its callers reach.

Three costs follow from that.

- **Every caller hand-rolls HTTP.** An agent that wants a tool has to know the
  gateway's bespoke envelope, and know in advance which `mcp` and which `tool` it
  wants, because there is nothing to ask.
- **Discovery does not exist.** The registry knows every tool every registered
  MCP advertises ([ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md)),
  and the policy engine knows which of them a given caller may invoke
  ([ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md)). Neither
  fact has anywhere to go. A caller discovers it is unauthorized by calling and
  getting a 403.
- **The asymmetry is now conspicuous.**
  [ADR-0017](0017-http-and-sse-transports-for-downstream-mcps.md) taught the
  gateway to *reach* an MCP over `stdio`, `http` or `sse`.
  [ADR-0011](0011-mcp-control-server.md) put a real MCP server on the *control*
  plane, at `/admin/mcp`, so an operator can administer the gateway through MCP.
  The one surface that is supposed to serve tools to agents is the one that
  cannot.

The vendored `github.com/modelcontextprotocol/go-sdk` v1.7.0 already ships both
server transports — `NewStreamableHTTPHandler` and `NewSSEHandler` — and ADR-0011
already uses the first of them. So this is wiring and policy integration, not
protocol work.

## Decision

**Serve the gateway itself as an MCP server on the data-plane listener, over
both HTTP transports**, alongside the existing `POST /v1/call`:

| Path | Transport |
|------|-----------|
| `/v1/mcp` | MCP Streamable HTTP — the current standard |
| `/v1/sse` | HTTP+SSE — the 2024-11-05 transport, still what many deployed clients speak |

Both are always on. They are authenticated exactly as `POST /v1/call` is, and
serving tools to agents is the data plane's purpose, so an opt-out would be a
config key guarding nothing.

### The tool surface is the whole registry, namespaced and filtered

`tools/list` returns one entry per `(mcp, tool)` pair, named `<mcp>__<tool>`,
for every registration that is active and enabled — **and only those pairs the
caller's access policy authorizes.** A caller never sees a tool it could not
invoke. This is the first time the policy engine's access decision is visible to
a caller as anything other than a 403 on a call it should not have made.

`tools/call` splits the qualified name and runs the **existing** pipeline:
auth → authorize → route → call → filter, the same `runPipeline` that
`POST /v1/call` runs, returning the same filtered payload. This is deliberate and
is the same reasoning ADR-0011 applied to `adminservice`: two implementations of
"call a tool subject to policy" is a latent correctness bug, and the one that
drifts will be the one nobody is looking at.

Name splitting resolves against the registry rather than by parsing: a qualified
name matches the registration whose name is a prefix of it followed by `__` and
which actually advertises the remainder as a tool. Splitting on the first or last
`__` would silently mis-route whenever an MCP name or a tool name contains the
separator.

### The sdk's net/http handlers mount on fasthttp through the adaptor

The sdk's handlers are `net/http`; the data plane is fasthttp. `fasthttpadaptor`
in fasthttp v1.73.0 watches for the first `Flush` and switches from buffering the
response to `SetBodyStreamWriter`, so a handler that streams — which is what both
of these do — streams through it. That is what makes this a route table entry
rather than a third listener to operate, document and secure.

### Claims bind at session establishment; authorization is re-evaluated per call

The sdk populates `RequestExtra.Header` — the per-request HTTP headers a tool
handler can read — **only on the streamable transport.** `sse.go` never sets it,
and the sdk's own SSE client does not resend `Authorization` on the message
POSTs it makes to the session endpoint. There is therefore no mechanism that
gives both transports a fresh token per JSON-RPC message.

So the bearer token is validated and its claims bound **once, at session
establishment**, through the sdk's `getServer(*http.Request)` callback: the SSE
hanging GET, and the first request of a streamable session. That is uniform
across both transports, which matters more here than squeezing an extra check
out of the one transport that could support it.

Two things keep that from being a weaker gate than `POST /v1/call`:

- **Authorization is not snapshotted.** Every `tools/list` and every `tools/call`
  re-runs `Authorize` against the live policy engine. Revoking a grant takes
  effect on the caller's next request, with no reconnect — only the caller's
  *identity* is fixed for the session, not what that identity may do.
- **Token validity is still enforced per request.** Every request to these paths
  that carries an `Authorization` header has it validated, and one is required to
  establish a session. A streamable client resends it on every request, so an
  expired token is rejected there on the next message.

The residual gap is an SSE session whose message POSTs carry no header: it
remains usable until the stream drops, authorized by possession of an
unguessable session id that was only ever handed to an authenticated GET. This
is written down in `docs/api/data-plane.md` rather than papered over.

### The two transports need different session contexts, and neither is obvious

`connectStreamable` stores `req.Context()` in a `ServerSession` that **outlives
the POST that created it.** Under `fasthttpadaptor` that context is the
`*fasthttp.RequestCtx` — an object fasthttp pools, resets, and hands to an
unrelated connection as soon as the handler returns. A live session reading
values from it would be racing another request's reset. The streamable handler
is therefore given a context detached from the request and rooted at the
gateway's own lifetime.

The SSE handler does the opposite and keeps the request context, because
`SSEHandler.ServeHTTP` blocks on `req.Context().Done()` for the session's entire
life. fasthttp releases a `RequestCtx` only after the response body stream
completes, so it stays valid throughout — and that context is what lets the
handler return at all.

The awkward part is what `RequestCtx.Done()` actually means. It returns the
*server's* done channel, not the request's, because fasthttp will not allocate
a channel per request; it closes on shutdown and at no other time. So **neither
transport learns from its context that a client hung up.** An abandoned SSE
session would sit in `ServeHTTP` until the gateway stopped.

The sdk's own keepalive is the answer: the server pings a connected client on an
interval and closes the session when the ping fails, which is exactly the signal
the context cannot give. It is set for both transports.

Shutdown falls out of the same facts. An open session holds a hanging GET, and
fasthttp's graceful shutdown waits for in-flight requests, so `Stop` ends the
sessions **before** calling `ShutdownWithContext` rather than after. Cancelling
afterwards — the obvious shape, as a `defer` — deadlocks until the shutdown
deadline expires, which is a process that ignores SIGTERM. There is a test for
exactly this.

### Tool descriptions become part of the schema

`mcp.ToolSchema` carried `Name`, `InputSchema` and `OutputSchema`. A downstream
tool's description was read from `tools/list` and thrown away, because nothing
consumed it: `POST /v1/call` needs a caller to already know what it wants.

An MCP client chooses a tool by its description, so this endpoint makes the field
load-bearing. `Description` is plumbed through `mcp.ToolSchema` to
`cache.ToolSchema` and populated everywhere tools are discovered — registration,
refresh, and health-check reconnect.

## Alternatives Considered

### Alternative A: Leave the data plane as it is; point clients at a bridge

Advantages:
- No change to the gateway.
- One request shape to reason about and secure.

Disadvantages:
- Every operator has to find, run and monitor an MCP↔REST bridge, which then
  sits inside the trust boundary holding the same payloads with none of the
  gateway's policy machinery. This is Alternative A of ADR-0017 again, one layer
  up, and it was wrong for the same reason.
- A bridge cannot implement `tools/list` correctly: the authorized tool set is a
  function of the caller's claims and the live policy engine, which only the
  gateway can evaluate.

### Alternative B: A separate net/http listener for the MCP endpoint

Advantages:
- No adaptor. The sdk's handlers run on the server they were written for, and
  the streaming and context-lifetime questions largely evaporate.
- The MCP surface could be bound to a different interface from `/v1/call`.

Disadvantages:
- A third listener to configure, document, secure, health-check and shut down
  gracefully. ADR-0011 declined exactly this for the control plane, and the
  reasoning holds here.
- Splits the data plane in two: the same callers, the same policy, the same
  auth, on two ports, for an implementation detail of which HTTP library each
  half uses.
- The adaptor turned out to support streaming, so the advantage is mostly
  theoretical.

### Alternative C: Implement the MCP transports natively in fasthttp

Advantages:
- No adaptor, no `net/http` in the data plane, full control over streaming via
  `SetBodyStreamWriter`.
- Fastest path, in principle, on the performance-critical surface.

Disadvantages:
- Hand-writing session management, SSE framing, resumability and the streamable
  transport's method/header protocol — and then owning them as the spec moves.
  ADR-0011's "reuse the existing dependency" applies with more force here, not
  less.
- The gateway's overhead budget (~1-2ms, ticket #11) is dominated by the
  downstream call, not by response framing.

### Alternative D: Streamable HTTP only, skip SSE

Advantages:
- One transport; SSE is being phased out upstream.
- Smaller surface to test, and the transport with the better per-request header
  story — `RequestExtra.Header` would make claims fresh per message.

Disadvantages:
- A large installed base of clients still speaks only SSE, which is precisely
  why ADR-0017 accepted it on the downstream side. Accepting SSE from servers
  while refusing it from clients is hard to justify.
- The sdk already provides the handler; the marginal cost is a route and a test.

### Alternative E: Expose one flat tool per MCP (`call_postgres_ro`) with the tool name as an argument

Advantages:
- No namespacing question, no name splitting.
- A stable tool list that does not change as downstreams come and go.

Disadvantages:
- Discards the input schemas, which is the main thing a client wants from
  `tools/list` — the model can no longer be shown what arguments a tool takes.
- Makes per-tool access policy invisible again: the tool list would be identical
  for every caller, and authorization would only surface as a failed call.

## Decision Criteria

1. Can an off-the-shelf MCP client connect to the gateway? (A: no.)
2. Does `tools/list` reflect *this caller's* authorized set, with schemas?
   (A: no. E: no.)
3. Is there exactly one implementation of "call a tool subject to policy"?
   (A: no.)
4. Does it work with clients that exist today, not only new ones? (D: no.)
5. Does it avoid a new listener and a hand-written transport? (B: no. C: no.)
6. Does `POST /v1/call` keep working unchanged? (All: yes — and it must.)

Only the chosen design satisfies all six.

## Rationale

The gateway's value is that a caller asks it for a tool and gets back exactly
what policy says it may have. Everything needed for that has been in place since
ADR-0004: the registry knows the tools, the engine knows the grants, the filter
knows the fields. What was missing was a way for a caller to *ask* in the
protocol it already speaks.

The parts worth deliberating were not the transports — the sdk has those — but
where the caller's identity comes from on a transport that supplies headers only
once, and whose context a session may safely hold. Both were settled by reading
what the sdk actually does rather than what it appears to do, and both are
written down above because neither is visible from the call site.

## Consequences

### Positive

- An MCP client can point at the gateway and see a tool catalogue filtered to
  what its token authorizes, with full input and output schemas.
- Access policy becomes discoverable instead of only enforceable: two callers
  with different grants see different tool lists.
- `POST /v1/call` and `tools/call` share one pipeline, so filtering, disable
  semantics and every error mapping stay identical by construction.
- Tool descriptions survive discovery, which also improves the admin UI and the
  ADR-0011 `list_mcps` tool.
- The demo stack can register MCP Inspector against the gateway itself, not only
  against a downstream.

### Negative

- The data plane now depends on `fasthttpadaptor` and, through it, runs
  `net/http` handlers. The adaptor's streaming path is newer than its buffering
  path and is now load-bearing for SSE.
- The gateway holds long-lived sessions for the first time. Their liveness rests
  on a keepalive ping rather than on a context, because fasthttp's request
  context cannot report a client disconnect.
- A caller's identity is fixed for a session's lifetime. An SSE client whose
  message POSTs omit `Authorization` keeps a session usable past token expiry
  until the stream drops.
- Two ways to call a tool is two things to document and keep in step, even
  sharing one pipeline.
- Tool names are namespaced, so a client sees `postgres-ro__get_user` rather
  than `get_user`. An MCP whose name contains `__` is still reachable, because
  splitting resolves against the registry, but the names get harder to read.

### Neutral

- MCP prompts and resources are not exposed, matching ADR-0011's scope.
- `tools/list` is returned in a single page. The registry is an in-memory map
  and the whole catalogue is already materialized to filter it, so paginating
  would add a cursor to serialize for no saving.
- Server-initiated notifications such as `tools/list_changed` are not sent.
  ADR-0013's polling remains how schema freshness is handled; a client sees a
  changed catalogue on its next `tools/list`.
