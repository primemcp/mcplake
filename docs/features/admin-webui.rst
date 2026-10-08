Admin Web UI
============

A React 19 + TypeScript SPA for operating the gateway without hand-writing
``curl`` calls against the :doc:`Control-Plane API </reference/admin-api>`: registering MCP
endpoints and managing their response filters. Embedded into the control-plane
binary and served from the same listener/port — no separate origin, no CORS
configuration, nothing extra to deploy.

Source: ``gateway/internal/controlplane/webui/``.

Running it
----------

**Production (embedded):** built automatically as part of ``make build`` (or
standalone via ``make ui-build``, which writes ``webui/dist``, embedded by
``gateway/internal/controlplane/webui.go`` via ``//go:embed``). The compiled
gateway binary serves it directly — nothing to run separately.

Container (``deploy/webui/Dockerfile``): a prebuilt image of its own, whose
final layer is ``gcr.io/distroless/static`` — the SPA is compiled during the
image build and the running container only serves it. The compose demo brings
it up as the ``webui`` service on port 8082. See
:doc:`ADR-0020 </architecture/decisions/0020-serve-the-admin-ui-from-its-own-container>`.

That container is also the browser's single origin: it serves the assets **and**
reverse-proxies ``/admin/*`` to the gateway's control plane, because the UI calls
the admin API with relative URLs and ADR-0014's PKCE redirect names the origin
you browsed to. Its settings, all overridable by flag or environment variable:

.. list-table::
   :header-rows: 1

   * - Flag
     - Env
     - Default
     - Purpose
   * - ``-addr``
     - ``WEBUI_ADDR``
     - ``:8081``
     - Listen address.
   * - ``-root``
     - ``WEBUI_ROOT``
     - ``/srv/www``
     - Directory holding the
       built SPA. Startup fails
       if it has no
       ``index.html``.
   * - ``-api-prefix``
     - ``WEBUI_API_PREFIX``
     - ``/admin``
     - The single path prefix
       forwarded to the gateway.
       Nothing else is proxied.
   * - ``-api-target``
     - ``WEBUI_API_TARGET``
     - *(empty)*
     - Gateway control-plane base
       URL, e.g.
       ``http://localhost:9091``.
       Empty disables proxying
       entirely — the right
       setting when something in
       front already routes
       ``/admin/*``; the server
       logs a warning so it is
       not a silent
       misconfiguration.

The two ways of serving the UI differ in exactly one behaviour: a request for a
*missing asset* (a path with a file extension) returns ``404`` here, where the
embedded server returns ``index.html``. Returning the shell for
``/assets/index-abc123.js`` hands the browser HTML with a JavaScript content type,
which shows up as a syntax error rather than a clear miss.

**Local frontend development:** ``make ui-dev`` starts Vite's dev server, which
proxies ``/admin`` to a gateway control-plane already running locally (default
``http://localhost:8081`` — see ``vite.config.ts``), so the dev server talks to
the real admin API instead of a mock. Start the gateway first
(``make build && ./cmd/gateway/mcp-gateway --config <your config>``), then
``make ui-dev`` in a second terminal.

**Tests:** ``make ui-test`` (``bun install --frozen-lockfile && bun run test``) —
Vitest + React Testing Library, no live gateway required (each test mocks
``fetch`` directly, see ``src/api/client.test.ts`` for the pattern). Type-checking
runs as part of ``make ui-build`` (``tsc -b && vite build``).

Screens
-------

MCP connections
~~~~~~~~~~~~~~~

Endpoint list (left column) + detail/edit panel + response filters, for the
selected endpoint.

- **URL-addressed.** Selecting an endpoint pushes ``/mcps/:name`` (the gateway
  serves the SPA for any path outside ``/admin/*`` — ``RegisterUIRoutes``'s
  ``NoRoute`` handler already did this, so no backend change was needed to
  add it); loading that URL directly opens the UI with that endpoint
  pre-selected. ``/users`` is the other addressed screen. Everything else
  (including bare ``/``) falls back to whichever screen localStorage
  remembers, same as before — see ``lib/route.ts``/``state/useNav.ts``. Browser
  back/forward moves between both screens and between endpoints. Kept
  deliberately dependency-free (hand-rolled ``history.pushState``/``popstate``,
  not a router library) — two screens and one deep-linkable detail page
  don't justify one, the same reasoning as ``auth/oidc.ts``'s hand-written
  PKCE.

- **Add/edit an endpoint** registers via ``POST /admin/mcps`` (there is no
  ``PUT`` — editing re-registers under the same name, which the backend
  upserts). Every save carries the endpoint's current ``enabled`` flag
  explicitly (never left to the request's default-to-``true``) so editing an
  endpoint's connection details on a disabled one doesn't silently
  re-enable it.

- **Transport is a real choice.** All three of ``stdio``, ``http`` and ``sse``
  are selectable and all three work
  (:doc:`ADR-0017 </architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps>`);
  the picker used to render ``sse``/``http`` as permanently disabled buttons
  captioned "Not implemented by the gateway yet", which was accurate until
  the backend implemented them.

  The form shows the fields the selected transport actually uses — command

  - arguments for ``stdio``, a URL for ``http``/``sse`` — and sends **only**
    those. That matters: the backend rejects a ``url`` on a stdio entry and a
    ``command`` on a remote one outright, rather than ignoring the stray field,
    so carrying an abandoned value along would turn a correctly filled form
    into a ``400``. Both sets stay in form state while you are deciding, so
    flipping between transports doesn't eat what you already typed; the drop
    happens at the request boundary (``lib/transport.ts``'s ``buildConnect``).

  The URL field states the endpoint rule up front — https, or http only on
  a loopback host — rather than making you submit to discover it. That is a
  **hint, not a client-side check**: duplicating a security rule in
  TypeScript is how the two drift, so the gateway stays the single place
  that decides, and its refusal is rendered against the URL field instead
  of as a banner you have to map back onto the form yourself.

- **A remote endpoint shows its URL** wherever a stdio one shows its
  command line — in the list row, in the detail header, and in search. The
  detail panel reports the endpoint's real transport rather than the
  hardcoded ``stdio`` it printed before.

- **Enable/disable** is a real, reversible toggle on the detail panel
  (``PATCH /admin/mcps/:name``) — a disabled endpoint stays registered
  (connected, tools cached) but rejects every data-plane call with 403
  ``mcp_disabled`` until re-enabled; no reconnect either way. Reflected
  immediately in the endpoint list (a neutral gray dot, "disabled" instead
  of the real connection status) and in the detail panel's status line.
  **Delete endpoint** is the separate, unrecoverable action (drops the
  registration, its schema cache, and every filter/grant on it). It is a
  button in the detail panel's header, next to "Edit endpoint" and the
  toggle, with a two-step "Delete endpoint" → "Confirm delete"
  confirmation. A failed delete shows the server's message there rather
  than silently resetting.

- **Response filters** are built from the endpoint's discovered tool schemas
  (``GET /admin/mcps`` already returns each tool's ``output_schema`` — no
  separate schema-discovery call). The field picker merges every tool's
  fields into one searchable, toggleable list; a field can be toggled
  regardless of which tool it belongs to, and **one filter can span several
  tools** even though the backend's ``FilterPolicy`` is always single-tool —
  see :doc:`ADR-0008 </architecture/decisions/0008-frontend-only-multi-tool-filter-grouping>`
  for how that's reconciled into real per-tool records without any backend
  change. Filter names may not contain ``::`` (reserved for that grouping
  convention — enforced in the UI, not just documented).

- **Schema graph**: any field with nested children shows a ``[N]`` badge (N =
  total descendant fields, recursively) that opens an interactive
  node/edge graph of that tool's full response shape (``@xyflow/react`` +
  ``@dagrejs/dagre`` for layout). Clicking an edge toggles whether that field
  is dropped — the same underlying selection state as the flat list, just a
  more legible way to browse/edit a deeply nested or heavily branching
  schema than a flat list would allow.

Users & access
~~~~~~~~~~~~~~

User list (left column) + a detail panel with three tabs for the selected
user. A "user" here is UI composition, not a backend entity: it is an
``AccessPolicy`` plus every ``FilterPolicy`` whose name is grouped under that
policy's name (``<user>::<label>::<tool>``, the same ``::`` convention as
:doc:`ADR-0008 </architecture/decisions/0008-frontend-only-multi-tool-filter-grouping>`).
Listing users means ``GET /admin/access-policies`` joined with
``GET /admin/filter-policies`` by name; deleting one deletes all of those
records.

- **Token match** edits the shared claim conditions (``ClaimRule[]``: JSONPath

  - regex, AND-ed). Saving writes them to the ``AccessPolicy`` and re-PUTs every
    filter of that user whose match has drifted, so the records never
    disagree. At least one condition is required: a matcher with zero rules
    matches *every* token (see ``router/claimrule.go``).

- **Access** picks the endpoints (and optionally a tool subset) the user is
  granted, and the response filters attached to each grant. Grants save with
  the panel's Save button; filters save immediately as independent records.

- **Request path** simulates what the gateway does with one request from
  this user. The operator pastes/edits a decoded JWT payload and picks the
  target endpoint + tool; the tab walks the same stages, in the same order
  and with the same status codes, as the real data plane
  (``gateway/internal/toolcall.go``): payload not a JSON object → 401; no
  enabled ``AccessPolicy`` that both matches the claims and grants the call →
  403 ``forbidden``; endpoint disabled → 403 ``mcp_disabled`` (only reported
  after authorization, like the gateway); endpoint not active / tool
  unknown → 404; otherwise 200 plus the union of ``drop_fields`` from every
  enabled ``FilterPolicy`` targeting that endpoint + tool whose match holds.
  Every policy's and filter's own decision is shown ("matches · no grant on
  this call", "disabled · skipped", …), and rejections get a red badge and a
  pulsing alert on the node that rejected.

  It is a **simulation against the policies as currently saved** — computed
  entirely in the browser (``lib/jsonpath.ts``, ``lib/claimMatch.ts``,
  ``lib/requestPath.ts``), never sent to the gateway, and labeled as such in the
  UI. It is not a live request trace or an audit log, and the tab warns when
  the panel holds unsaved Token match / Access edits it can't see.

  Limitations: JWT signature, expiry and issuer are the real Auth
  Validator's job and are not checked. The JSONPath reimplementation covers
  names, wildcards, indexes, slices, unions and descendant segments; a rule
  using a filter selector (``[?...]``) is valid on the gateway but outside
  this subset, so the tab names it in a "could not be simulated" caveat,
  keeps evaluating every other policy, and does **not** claim the gateway
  would reject the request. Regexps are JavaScript ``RegExp`` rather than Go's
  RE2 — identical for the anchors/classes/groups/alternation policies
  normally use, different only for RE2-specific syntax.

  Claim values are stringified exactly as the gateway does, including Go's
  ``%g`` switch to scientific notation at 1e6 (``1000042`` → ``1.000042e+06``).
  ``testdata/claim_stringify_cases.json`` is the shared fixture both this
  implementation and ``router``'s Go one are tested against, so the two cannot
  drift apart unnoticed — they had, and the tab was reporting redactions the
  gateway never performed.

Data model notes specific to the UI
-----------------------------------

- **Schema flattening and search** (``lib/schema.ts``) recurses through nested
  objects and arrays-of-objects using the same JSONPath ``[*]`` wildcard
  syntax ``filter.Strip()`` actually matches against (e.g.
  ``$.entities[*].name``) — verified against a real 15-level, branching test
  fixture, not just shallow examples. Arrays of primitives/arrays are
  leaves (not expandable), described recursively (``array<array<string>>``).
- The ``[N]`` badge (``descendantCount``) counts every field nested under a
  row, recursively — not how many levels deep the nesting goes. A row with
  two direct leaf children reads ``[2]``, the same as a row with one child
  that itself has one child.
- **Multi-tool filter grouping** (``lib/filterGroups.ts``): see
  :doc:`ADR-0008 </architecture/decisions/0008-frontend-only-multi-tool-filter-grouping>`.

Security
--------

A same-repo security audit (2026-09-05) of this feature found and fixed two
issues, both now covered by regression tests:

- A filter name containing ``::`` could silently merge a new filter into an
  unrelated existing filter's real records on save (see ADR-0008's Risks
  section) — fixed by rejecting ``::`` in the name input.
- The field picker's internal ``(tool, path)`` composite key was
  delimiter-joined with a plain space, which a tool name containing a space
  (nothing validates tool-name charset anywhere on the backend — see
  :doc:`ADR-0003 </architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery>`)
  would silently misparse, saving a filter against a tool that doesn't
  exist — fixed by JSON-encoding the pair instead of delimiter-joining it.

Signing in
~~~~~~~~~~

When the gateway enforces admin auth (``admin_auth.match`` — see
:doc:`ADR-0010 </architecture/decisions/0010-control-plane-admin-authentication>`),
the UI obtains its own token through an OIDC **Authorization Code + PKCE**
flow run in the browser, against the same provider ``[oidc]`` already verifies
tokens from. Configure it with
:ref:`[admin_auth.login] <reference-configuration-admin-ui-sign-in>`;
:doc:`ADR-0014 </architecture/decisions/0014-admin-ui-oidc-pkce-login>` records
why this shape.

- On load the UI reads ``GET /admin/auth/config`` (the one open admin route
  besides ``healthz``) to learn whether a sign-in is needed and where to go. No
  admin screen renders — and so no admin request fires — until that is
  settled.
- **Sign in** redirects to the provider with an S256 ``code_challenge``; the
  ``code_verifier`` stays in this tab. On return the ``state`` is checked against
  the one this tab stored, and ``?code=…&state=…`` is scrubbed from the address
  bar so a reload can't replay it. The check gates **both** outcomes — a
  success and a provider error — so a link someone sends the operator can
  neither hand the UI an authorization code nor render the sender's own text
  inside the gateway's sign-in card. An unverifiable callback also leaves a
  real sign-in that is in flight in the same tab untouched. Once verified, a
  provider error shows the provider's own words, which is what tells an
  operator whether to fix a client registration or simply retry.
- The token lives in ``sessionStorage``: it survives a reload, is not shared
  with other tabs, and is gone when the tab closes. It never goes into
  ``localStorage``, which is where the UI keeps only cosmetic state (selected
  screen/endpoint/user).
- An expired token is renewed silently when the provider issued a
  ``refresh_token``, once even if several screens ask at the same moment.
  Otherwise the operator returns to the sign-in screen.
- **401 and 403 are different screens.** A 401 means sign in again. A 403
  means the token is valid but its claims don't satisfy ``admin_auth.match``, so
  the UI says exactly that, names the signed-in subject, and offers only sign
  out — re-authenticating as the same person would loop forever.
- With admin auth **off**, none of this appears: no sign-in screen, no
  ``Authorization`` header, no identity in the sidebar.
- **A deep link survives sign-in.** ``redirectUri()`` is fixed to this
  origin's bare ``/`` (never taken from the current path — that's how open
  redirects happen), so the provider always sends the browser back to the
  root regardless of where sign-in started. The path at the moment "Sign
  in" is clicked (e.g. a bookmarked ``/mcps/:name``) is stashed alongside the
  PKCE ``verifier``/``state`` in ``auth/session.ts``'s ``Pending`` record and put
  back (``history.replaceState``, ``auth/browser.ts``'s ``restorePath``) once the
  code exchange succeeds — otherwise a shared/bookmarked MCP link would
  silently land on the endpoint list after signing in.
- Not covered: signature/``exp``/``iss`` checks (the gateway does those), and
  signing out of the provider itself — sign-out is local to this UI.

Bind the control-plane listener to a trusted network/interface regardless.
