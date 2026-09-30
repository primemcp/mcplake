MCP Servers
===========

The MCP servers behind the gateway are called **downstream MCPs**, and each
one is known to the gateway by a **registration**: a unique name, a transport,
and how to reach it.

Registering a server
--------------------

A registration comes from ``[[mcps]]`` in the configuration file, or at
runtime from the admin API, the admin web UI or the MCP control server. All
paths do the same thing: connect to the server, run the MCP handshake, ask for
its tools (``tools/list``) and cache them — names, descriptions, input and
output schemas.

Transports
----------

.. list-table::
   :header-rows: 1
   :widths: 12 30 58

   * - ``type``
     - Where the server runs
     - Registration needs
   * - ``stdio``
     - A subprocess the gateway starts on its own host (the default).
     - ``command``, optional ``arguments`` and ``env``.
   * - ``http``
     - Anywhere reachable over the network, speaking MCP Streamable HTTP.
     - ``url``
   * - ``sse``
     - Anywhere reachable, speaking the older HTTP+SSE transport.
     - ``url``

.. code-block:: toml

   [[mcps]]
   name = "filesystem"
   type = "stdio"
   command = "mcp-server-filesystem"
   arguments = ["/data"]

   [[mcps]]
   name = "crm"
   type = "http"
   url = "https://mcp.crm.internal/mcp"

Rules worth knowing:

- A ``url`` must be ``https``, or ``http`` only to a loopback host: tool
  arguments and results are exactly the data the gateway exists to protect.
- There is no outbound authentication yet, so a network MCP that requires
  credentials cannot be registered.
- A ``stdio`` server does **not** inherit the gateway's environment. It gets
  only ``PATH``, ``HOME``, ``LANG``, ``LC_ALL``, ``TZ`` and ``TMPDIR`` (when
  set), plus whatever its own ``env`` table declares — so the gateway's own
  secrets never leak into a tool.
- Registering a ``stdio`` server runs a command on the gateway host. That is
  why the control plane must be protected like shell access; see
  :doc:`authentication`.

One catalogue, namespaced tools
-------------------------------

Callers connected over MCP see one flat tool list built from every
registration, with each tool named ``<mcp>__<tool>`` — the registration name,
two underscores, the server's own tool name:

.. code-block:: text

   crm__search_accounts
   crm__get_account
   filesystem__read_file
   employee-directory__get_employee

The prefix keeps two servers that both offer ``search`` apart, and tells the
caller where each tool comes from. Descriptions and schemas are passed through
unchanged. Policies and ``POST /v1/call`` use the plain names (``mcp`` and
``tool`` separately).

Name registrations for what they are and who they are for —
``postgres-ro`` and ``postgres-rw`` for two instances of the same server with
different database users is a common pattern.

Status and self-healing
-----------------------

Every registration has a status:

.. list-table::
   :widths: 20 80

   * - ``connecting``
     - Being connected and discovered.
   * - ``active``
     - Connected; its tools are callable.
   * - ``unreachable``
     - Connection or discovery failed, or the connection was lost. Calls fail
       with ``503 mcp_unavailable``; its tools disappear from ``tools/list``.

A registration that fails is **kept**, not dropped. A health-check loop
(every ``30s`` by default) pings each active server and reconnects any whose
session died, and retries unreachable ones with a backoff that doubles up to
five minutes. So a server that restarts — or that was not up yet when the
gateway started — heals by itself. See :doc:`/features/mcp-health-check`.

Keeping tool schemas current
----------------------------

Tools are discovered at registration. If your servers add or change tools
while running, set ``[mcp] schema_refresh_interval`` and the gateway re-lists
them periodically on the live connection; see
:doc:`/features/mcp-schema-refresh`.

Switching a server off
----------------------

Every registration has an ``enabled`` flag. A disabled MCP stays connected and
keeps its cached tools, but every call to it is refused with
``403 mcp_disabled`` and its tools leave the MCP catalogue — a reversible kill
switch for a misbehaving server. See :doc:`/features/enable-disable`.

Reference
---------

- :ref:`reference-configuration-4-mcp-servers` and
  :ref:`reference-configuration-mcp-global` — configuration keys.
- :doc:`/reference/admin-api` — registering, updating and removing at runtime.
- :doc:`/architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery`,
  :doc:`/architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps`,
  :doc:`/architecture/decisions/0018-stdio-mcp-subprocesses-get-a-minimal-base-environment`.
