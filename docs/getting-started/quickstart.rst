Quickstart
==========

Run the complete demo stack — Keycloak as the identity provider, the gateway,
and several MCP servers — and see the three things the gateway does:
**authenticate** a caller, **authorize** a tool call from their claims, and
**filter** fields out of the response.

This takes about five minutes, most of it image downloads. For the full tour
of the stack, see :doc:`demo`.

Prerequisites
-------------

- Docker with the Compose plugin, or Podman with ``podman compose``.
- ``git``, ``curl`` and ``jq``.
- Free local ports ``8080`` (Keycloak), ``8082`` (admin UI), ``9090``
  (gateway data plane) and ``9091`` (gateway control plane).

1. Start the stack
------------------

.. code-block:: bash

   git clone https://github.com/atsokha/mcplake.git
   cd mcplake
   docker compose up -d --build
   # or: podman compose up -d --build

The first start builds the gateway image and imports the Keycloak realm. When
this prints ``ok``, the gateway is up:

.. code-block:: bash

   curl -s http://localhost:9090/healthz

The realm has three users, each with a ``role`` claim:

.. list-table::
   :header-rows: 1

   * - User / password
     - ``role`` claim
     - What the gateway's policies give them
   * - ``alice`` / ``alice``
     - ``db-reader``
     - The ``employee-directory`` MCP, with salary and SSN fields removed
   * - ``bob`` / ``bob``
     - ``guest``
     - Nothing
   * - ``mcplake-admin`` / ``mcplake-admin``
     - ``admin``
     - Every MCP, unfiltered, plus the admin API and web UI

2. Call a tool as alice
-----------------------

Get a token from Keycloak, then call ``get_employee`` through the gateway:

.. code-block:: bash

   ALICE_TOKEN=$(curl -s -X POST http://localhost:8080/realms/mcplake/protocol/openid-connect/token \
     -d grant_type=password -d client_id=mcplake-demo \
     -d username=alice -d password=alice -d scope=openid | jq -r .access_token)

   curl -s -X POST http://localhost:9090/v1/call \
     -H "Authorization: Bearer $ALICE_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"mcp":"employee-directory","tool":"get_employee","arguments":{"employee_id":2}}' \
     | jq .structuredContent

You get Marcus Webb's record — without ``salary_usd`` and ``ssn_last4``. The
MCP server returned them; a :doc:`filter policy </concepts/response-filtering>`
matching alice's ``role`` removed them from every copy of the payload in the
response.

3. The same call as bob
-----------------------

.. code-block:: bash

   BOB_TOKEN=$(curl -s -X POST http://localhost:8080/realms/mcplake/protocol/openid-connect/token \
     -d grant_type=password -d client_id=mcplake-demo \
     -d username=bob -d password=bob -d scope=openid | jq -r .access_token)

   curl -s -w '\n%{http_code}\n' -X POST http://localhost:9090/v1/call \
     -H "Authorization: Bearer $BOB_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"mcp":"employee-directory","tool":"get_employee","arguments":{"employee_id":2}}'

``403 forbidden``. Bob's token is valid, but no
:doc:`access policy </concepts/access-policies>` grants ``role=guest`` this
tool.

4. Look at it as the operator
-----------------------------

Open http://localhost:8082/ and sign in as ``mcplake-admin`` /
``mcplake-admin``. The admin web UI shows the registered MCP servers, their
tools and status, and the access and filter policies you just exercised — and
lets you change them. A change takes effect on the next call, with no
restart.

5. Connect an MCP client
------------------------

The gateway is itself an MCP server. Point any MCP client that can send a
bearer token at the data plane:

.. code-block:: javascript

   {
     "type": "http",
     "url": "http://localhost:9090/v1/mcp",
     "headers": { "Authorization": "Bearer <alice's token>" }
   }

The client's tool list contains only what alice's token is granted, named
``<mcp>__<tool>`` — for example ``employee-directory__get_employee``. See
:doc:`/reference/data-plane-api` for the protocol details.

Stop or reset
-------------

.. code-block:: bash

   docker compose down        # stop, keep data
   docker compose down -v     # stop and delete the gateway's database

.. tip::

   After pulling a newer revision, restart with
   ``docker compose down -v && docker compose up -d --build``. The gateway
   seeds each configured entry into its database only once
   (:doc:`/concepts/configuration-and-state`), so an old volume keeps the old
   policies.

Troubleshooting
---------------

The token request returns ``"Account is not fully set up"``
   Keycloak 26 requires a user-profile step for the demo users' ``role``
   attribute. This is a known demo issue tracked in :issue:`209`.

``curl`` to ``:9090`` is refused
   The gateway waits for Keycloak and Postgres before it starts. Watch
   ``docker compose logs -f gateway``; it starts within a minute or two of
   the first boot.

Next steps
----------

- :doc:`first-gateway` — the same setup with your own identity provider and
  MCP server.
- :doc:`/concepts/how-it-works` — what happened inside the gateway on each of
  those calls.
