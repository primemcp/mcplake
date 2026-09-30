Manage MCP Servers
==================

Register, inspect, update, switch off and remove downstream MCP servers. All
of it can be done at runtime; the examples use the admin REST API, and the
admin web UI and the :doc:`MCP control server </reference/admin-mcp>` offer
the same operations.

The examples assume ``$ADMIN`` is the control-plane URL
(``http://127.0.0.1:8081/admin``) and ``$ADMIN_TOKEN`` an administrator's
token. Drop the ``Authorization`` header if ``admin_auth`` is not configured.

Register a server
-----------------

**stdio** — a process the gateway starts:

.. code-block:: bash

   curl -s -X POST $ADMIN/mcps \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "filesystem",
          "transport": "stdio",
          "connect": {"command": "/usr/local/bin/mcp-server-filesystem",
                      "arguments": ["/srv/shared"],
                      "env": {"LOG_LEVEL": "warn"}}}'

**http** or **sse** — a server that is already running:

.. code-block:: bash

   curl -s -X POST $ADMIN/mcps \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "crm", "transport": "http",
          "connect": {"url": "https://mcp.crm.internal/mcp"}}'

The gateway connects and lists the server's tools before answering. ``201``
returns the registration with its tools. ``502 registration_failed`` means it
could not connect — the registration is still **saved as** ``unreachable`` and
retried by the health check, so fix the server (or the URL) rather than
registering again.

.. note::

   The REST API and the MCP control server call the transport ``transport``;
   the configuration file calls it ``type``.

Registering a server grants nobody anything: add an
:doc:`access policy </concepts/access-policies>` for it.

Inspect servers
---------------

.. code-block:: bash

   curl -s $ADMIN/mcps -H "Authorization: Bearer $ADMIN_TOKEN" \
     | jq '.[] | {name, transport, status, enabled, tools: (.tools | keys)}'

``status`` is ``active``, ``connecting`` or ``unreachable``; see
:doc:`/concepts/mcp-servers`. The admin web UI shows the same, with each
tool's schema.

Change a server's command, arguments or URL
-------------------------------------------

Register it again under the same name: the registration is replaced, and the
gateway reconnects and rediscovers its tools. Policies refer to the name, so
they keep applying.

Switch a server off and on
--------------------------

.. code-block:: bash

   curl -s -X PATCH $ADMIN/mcps/crm \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"enabled": false}'

Effective on the next call: calls fail with ``403 mcp_disabled`` and the tools
leave every caller's catalogue. The connection and cached tools are kept, so
``{"enabled": true}`` restores it instantly.

Remove a server
---------------

.. code-block:: bash

   curl -s -X DELETE $ADMIN/mcps/crm -H "Authorization: Bearer $ADMIN_TOKEN"

The registration, its connection and its cached tools are dropped. Policies
that name it are left as they are — delete or edit them separately. A server
seeded from ``config.toml`` is not re-created by a restart.

Names containing ``/``
----------------------

Names may contain ``/``. In URL paths, encode it: ``DELETE $ADMIN/mcps/team%2Fcrm``
for a server named ``team/crm``.

See also
--------

- :doc:`/reference/admin-api` — full request and response details.
- :ref:`reference-configuration-4-mcp-servers` — declaring servers in the
  configuration file.
