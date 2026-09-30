Data-Plane API
==============

The data-plane is the fasthttp server described in
:doc:`ADR-0001 </architecture/decisions/0001-use-fasthttp-for-gateway-server>`.
It listens on ``server.data_plane_addr`` (see :doc:`/reference/configuration`) and
is the only surface agents talk to — never the control-plane admin API, never a
downstream MCP directly.

``GET /healthz``
----------------

Returns ``200 OK`` with body ``ok`` once the gateway has bound its listener. No
authentication required.

``POST /v1/call``
-----------------

Invokes a tool on a registered downstream MCP, subject to the caller's JWT-derived
access policy (see :doc:`ADR-0004 </architecture/decisions/0004-unified-policy-engine-for-access-and-filtering>`).

Request
~~~~~~~

::

   POST /v1/call
   Authorization: Bearer <jwt>
   Content-Type: application/json

   {
     "mcp": "postgres-ro",
     "tool": "get_user",
     "arguments": { "id": 42 }
   }

============= ====== ======== =========================================
Field         Type   Required Description
============= ====== ======== =========================================
``mcp``       string yes      Registered MCP name to route the call to.
``tool``      string yes      Tool name to invoke on that MCP.
``arguments`` object no       Passed through to the MCP tool call.
============= ====== ======== =========================================

The ``Authorization`` header is required and must use the ``Bearer`` scheme.

The full pipeline (auth → authorize → route → call → filter) is described in
:ref:`architecture-data-request-lifecycle`.

Responses
~~~~~~~~~

.. list-table::
   :header-rows: 1

   * - Status
     - Body ``error`` code
     - When
   * - 200
     - —
     - Tool call succeeded; body is the tool result envelope, with every ``drop_fields`` path removed from each copy of the payload it
       carries.
   * - 400
     - ``invalid_json``
     - Request body is not valid JSON.
   * - 400
     - ``missing_field``
     - ``mcp`` or ``tool`` is missing/empty.
   * - 401
     - ``missing_authorization``
     - ``Authorization`` header is absent.
   * - 401
     - ``invalid_authorization``
     - Header present but not a (non-empty) ``Bearer`` token.
   * - 401
     - ``unauthorized``
     - Token present but fails signature/exp/iss/aud validation.
   * - 403
     - ``forbidden``
     - Token valid but claims don't authorize this ``(mcp, tool)`` (no ``AccessPolicy`` grants it).
   * - 404
     - ``mcp_not_found``
     - No MCP is registered under that name.
   * - 404
     - ``tool_not_found``
     - ``mcp`` exists but doesn't advertise ``tool``.
   * - 503
     - ``mcp_unavailable``
     - ``mcp`` is registered but has no live session right now — its downstream is down or restarting. The health-check loop is already
       retrying it, so this is worth retrying; see
       :repo:`mcp-health-check.md <docs/features/mcp-health-check.md>`.
   * - 502
     - ``upstream_error``
     - The downstream MCP call itself failed (connection issue, tool-level error).
   * - 502
     - ``filter_unenforceable``
     - A filter policy applied to this call, but the tool answered with free-form text its ``drop_fields`` cannot be applied to. The
       gateway refuses to return a body it could not redact — see
       :doc:`ADR-0015 </architecture/decisions/0015-filter-the-tool-payload-not-the-transport-envelope>`.
   * - 504
     - ``upstream_timeout``
     - The downstream MCP call didn't complete within the gateway's call timeout (``internal.Config.CallTimeout``, default 30s — not yet
       exposed as a ``config.toml`` key).
   * - 500
     - ``internal_error``
     - The Policy Engine or response filter failed unexpectedly — not a caller error.
   * - 501
     - ``not_implemented``
     - The gateway was started without a configured auth/policy/MCP pipeline (should not happen in a real deployment).

Every error body has the shape ``{"error": "<code>", "message": "<human-readable>"}``.

``404 mcp_not_found`` and ``503 mcp_unavailable`` are deliberately distinct. The
first means the registration does not exist and a caller should stop; the second
means it exists and its downstream is currently unreachable, which the gateway is
already working on (:doc:`ADR-0019 </architecture/decisions/0019-reconnect-downstream-mcps-on-a-health-check-loop>`).

Authorization is checked *before* existence: a caller lacking a grant for a given
``(mcp, tool)`` gets 403 even if that ``mcp``/``tool`` doesn't actually exist, matching the
``JWT eval -> [mcps] -> [tools]`` pipeline order in
:ref:`architecture-overview-pipeline-per-request` — access policies are
evaluated purely against claims and the requested names, independent of whether the
registry currently has anything registered under them.

.. _reference-data-plane-api-current-limitations-tracked-not-bugs:

Current limitations (tracked, not bugs)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- No request size limit or rate limiting is documented yet.

MCP endpoints: ``/v1/mcp`` and ``/v1/sse``
------------------------------------------

The gateway is itself an MCP server. Any MCP client — an agent, an MCP-enabled
editor, MCP Inspector — can connect to it directly instead of hand-rolling
``POST /v1/call``, and gets a tool catalogue filtered to what its token
authorizes. See
:doc:`ADR-0021 </architecture/decisions/0021-serve-mcp-on-the-data-plane>`.

.. list-table::
   :header-rows: 1

   * - Path
     - Transport
     - Methods
   * - ``/v1/mcp``
     - MCP Streamable HTTP — the
       current standard
     - ``POST``, ``GET``,
       ``DELETE``
   * - ``/v1/sse``
     - HTTP+SSE — the older
       transport, for clients
       that speak only it
     - ``GET`` (opens the
       session), ``POST``
       (messages)

Both are always enabled. There is no config key for them: they carry the same
authentication as ``POST /v1/call``, which is the only thing an opt-out would be
protecting.

Connecting
~~~~~~~~~~

.. code-block:: javascript

   // An MCP client's server entry
   {
     "type": "http",
     "url": "https://gateway.example.com/v1/mcp",
     "headers": { "Authorization": "Bearer <jwt>" }
   }

``"type": "sse"`` with ``.../v1/sse`` works the same way for a client that needs it.

``tools/list``
~~~~~~~~~~~~~~

Returns one tool per ``(mcp, tool)`` pair, named **``<mcp>__<tool>``** — the
registered MCP's name, two underscores, the tool's own name. Every registered
MCP's catalogue is flattened into one list, so the prefix is what keeps two
downstreams that both advertise a ``query`` apart.

A pair appears only when **all** of the following hold:

- the MCP's registration is ``active`` (its downstream is reachable),
- the MCP is not administratively disabled, and
- the caller's access policy grants that ``(mcp, tool)`` pair.

That last point is the difference from ``POST /v1/call``: **the catalogue is the
access policy, made visible.** Two callers with different claims see different
tool lists, and a caller is never offered a tool whose only possible answer is
``403``. An unauthorized caller sees an empty list rather than an error.

The list is returned in a single page, sorted by MCP name then tool name. Input
and output schemas, and each tool's description, are passed through from the
downstream unchanged.

``tools/call``
~~~~~~~~~~~~~~

Runs the same pipeline as ``POST /v1/call`` — auth → authorize → route → call →
filter — including every ``drop_fields`` path in the caller's filter policies. The
two surfaces share one implementation, so a response filtered on one is filtered
identically on the other.

Failures split two ways, following the MCP spec's distinction between a call
that could not be made and a call that failed:

.. list-table::
   :header-rows: 1

   * - Outcome
     - Conditions
   * - **JSON-RPC error**
     - Unknown tool, no grant for the
       pair (``forbidden``), MCP
       disabled (``mcp_disabled``), MCP
       or tool not found. The call
       should not have been made.
   * - **Result with
       ``isError: true``**
     - The downstream was reached and
       failed (``upstream_error``),
       timed out
       (``upstream_timeout``), is
       currently unreachable
       (``mcp_unavailable``), or
       answered something a filter
       policy could not be applied to
       (``filter_unenforceable``). The
       text content carries the same
       error code ``POST /v1/call``
       would have returned.

Authentication
~~~~~~~~~~~~~~

Every request to these paths that carries an ``Authorization: Bearer <jwt>``
header has it validated, and one is **required** to establish a session: the
hanging ``GET`` for SSE, and the first request of a streamable session. A request
without one is answered ``401`` with the same ``{"error","message"}`` body the rest
of the data plane uses.

Two properties are worth being precise about, because they are not the same:

- **Authorization is re-evaluated on every request.** ``tools/list`` and
  ``tools/call`` both consult the live policy engine, so revoking a grant or
  disabling an MCP takes effect on the caller's next call — no reconnect needed.
- **Identity is bound when the session is established.** The MCP Go SDK supplies
  per-request HTTP headers only on the streamable transport, and its SSE client
  does not resend ``Authorization`` on message ``POST``\ s, so there is no mechanism
  that would re-derive claims per message on both transports.

The consequence, stated plainly: an **SSE** session whose message ``POST``\ s carry
no ``Authorization`` header stays usable past the token's expiry, until the stream
drops. It is authorized by possession of the unguessable session id that was
handed only to an authenticated ``GET``. A **streamable** client resends the
header on every request, so an expired token is rejected there on the next
message. Deployments that need expiry enforced strictly should prefer
``/v1/mcp``.

Session lifetime
~~~~~~~~~~~~~~~~

The gateway pings a connected client every 30s and closes the session when the
ping fails, which is how a client that vanished without closing is reaped.
Sessions are also closed when the gateway shuts down.

Not exposed
~~~~~~~~~~~

- **MCP prompts and resources.** Tools only, matching the control plane's MCP
  server (:doc:`ADR-0011 </architecture/decisions/0011-mcp-control-server>`).
- **``notifications/tools/list_changed``.** Schema freshness is handled by polling
  (:doc:`ADR-0013 </architecture/decisions/0013-periodic-mcp-schema-refresh>`); a
  client sees a changed catalogue on its next ``tools/list``.
- **The admin API.** That is the control plane's ``/admin/mcp``, a separate
  listener and a separate credential.
