How It Works
============

mcplake is a reverse proxy that understands MCP. Agents never talk to an MCP
server directly: they talk to the gateway, the gateway talks to the servers,
and on the way through it decides — per caller, from their token — what is
allowed and what is visible.

Two planes
----------

The gateway runs two independent HTTP listeners.

.. list-table::
   :header-rows: 1
   :widths: 18 41 41

   * -
     - Data plane
     - Control plane
   * - Who uses it
     - Agents and applications
     - Operators, the admin web UI, automation
   * - Address
     - ``server.data_plane_addr``
     - ``server.control_plane_addr``
   * - Endpoints
     - ``/v1/mcp`` (MCP Streamable HTTP), ``/v1/sse`` (MCP over SSE),
       ``POST /v1/call`` (plain REST), ``/healthz``
     - ``/admin/*`` REST API, the embedded admin web UI, optionally
       ``/admin/mcp`` (the admin API as MCP tools)
   * - Authenticated by
     - Bearer JWT, always
     - Bearer JWT checked against ``admin_auth`` rules — when configured
   * - Does
     - Authorizes and proxies tool calls, filters responses
     - Registers MCP servers, manages policies, reports status

They share only the store underneath: the control plane writes registrations
and policies to the database and refreshes an in-memory cache; the data plane
reads that cache, so a change is live on the next call.

.. mermaid::

   flowchart LR
       Agent["Agent / MCP client"] -- "Bearer JWT" --> DP["Data plane<br/>:8080"]
       Op["Operator / admin UI"] -- "Bearer JWT" --> CP["Control plane<br/>:8081"]
       CP -- "write + refresh cache" --> Store[("SQLite / PostgreSQL")]
       DP -- "read (cached)" --> Store
       DP -- "stdio / HTTP / SSE" --> MCPs["Downstream<br/>MCP servers"]
       IdP["OIDC provider"] -. "JWKS" .-> DP
       IdP -. "JWKS" .-> CP

The life of a tool call
-----------------------

Every call — whether it arrives as MCP ``tools/call`` or ``POST /v1/call`` —
goes through the same pipeline:

.. mermaid::

   sequenceDiagram
       participant A as Agent
       participant G as Gateway
       participant M as MCP server
       A->>G: call (mcp, tool, args) + Bearer JWT
       G->>G: 1. validate JWT (signature, exp, iss, aud)
       G->>G: 2. access policies: is (mcp, tool) granted to these claims?
       G->>G: 3. is the MCP enabled and connected? does it have the tool?
       G->>M: 4. tools/call
       M-->>G: result
       G->>G: 5. filter policies: drop fields these claims may not see
       G-->>A: filtered result

1. **Authenticate.** The token's signature is checked against your OIDC
   provider's published keys (cached), along with its expiry, issuer and
   audience. Failure is ``401``.
2. **Authorize.** The token's claims are evaluated against every
   :doc:`access policy <access-policies>`. If none grants this exact
   ``(mcp, tool)``, the answer is ``403`` — before the gateway even looks up
   whether that MCP or tool exists, so a refusal reveals nothing.
3. **Route.** The named MCP must be registered, enabled and connected, and
   must advertise the tool.
4. **Call.** The gateway forwards the call over whatever transport that MCP
   uses — a stdio pipe to a subprocess, Streamable HTTP or SSE.
5. **Filter.** The claims are evaluated again, this time against
   :doc:`filter policies <response-filtering>` for this ``(mcp, tool)``, and
   every matching policy's fields are removed from the result.

Steps 2 and 5 are the same engine — :doc:`claim rules <claim-rules>` — applied
to two kinds of policy. That is the whole idea: *who you are*, as your
identity provider states it, decides both what you can call and what you get
back.

What an agent sees
------------------

An agent connected over MCP gets a single tool catalogue assembled from every
registered MCP server, with each tool named ``<mcp>__<tool>``. The catalogue
is filtered to exactly the pairs its token is granted, so two agents with
different claims see different tools, and neither is offered a tool it could
only get ``403`` from. See :doc:`mcp-servers`.

Design principles
-----------------

Claims are the single source of truth
   Who gets what is decided by what your identity provider puts in the token.
   Moving a user between groups there changes their access here, with no
   per-user state in the gateway.

Fail closed
   No matching grant means no access. A filter that cannot be applied to a
   response (for example free text instead of JSON) blocks the response
   rather than returning it unfiltered.

Deployable anywhere
   One Go binary, embedded SQLite by default, no telemetry and no outbound
   calls except to your OIDC provider's JWKS endpoint and the MCP servers you
   register — suitable for air-gapped networks.

Where the details are
---------------------

- :doc:`/architecture/overview` and :doc:`/architecture/data` — the same model
  from the implementation's side.
- :doc:`/reference/data-plane-api` — every status code the pipeline returns.
