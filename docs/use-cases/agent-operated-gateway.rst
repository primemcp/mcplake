Let an Agent Operate the Gateway
================================

The problem
-----------

Operators already work with MCP-capable assistants. Registering a new MCP
server, granting a team a tool, or switching off a misbehaving server
currently means the admin UI or hand-written API calls — and you would like to
ask for it in plain language, with the same access control as the admin API.

The approach
------------

Enable the **MCP control server**: every admin API operation exposed as an MCP
tool on the control-plane listener, at ``/admin/mcp``. It is protected by
``admin_auth`` exactly like the REST API, so only administrators' tokens can
use it.

.. code-block:: toml

   [admin_mcp]
   enabled = true
   # path = "/admin/mcp"     # default; must be under /admin/

   # Required in practice: without it the control plane is unauthenticated.
   [[admin_auth.match]]
   path = "$.groups[*]"
   pattern = "^mcplake-admins$"

Connect an MCP client to ``https://<gateway>:8081/admin/mcp`` with an
administrator's token, exactly as in :doc:`connect-mcp-clients`.

What the agent can do
---------------------

.. list-table::
   :header-rows: 1
   :widths: 35 65

   * - Tools
     - Operations
   * - ``gateway_health``
     - Check the control plane is up.
   * - ``list_mcps``, ``register_mcp``, ``set_mcp_enabled``, ``unregister_mcp``
     - Inspect registrations and their status, add a server, switch one
       off or on, remove one.
   * - ``list_access_policies``, ``get_access_policy``,
       ``create_access_policy``, ``replace_access_policy``,
       ``delete_access_policy``
     - Manage who may call what.
   * - ``list_filter_policies``, ``get_filter_policy``,
       ``create_filter_policy``, ``replace_filter_policy``,
       ``delete_filter_policy``
     - Manage field filtering.

Arguments follow the admin API's JSON shapes — with one naming difference:
``register_mcp`` takes the transport as ``transport`` (``"stdio"``, ``"http"``
or ``"sse"``), where the configuration file says ``type``. See
:doc:`/reference/admin-mcp` for every tool's input.

Example session
---------------

   *"The CRM MCP server is returning errors — switch it off until the vendor
   fixes it, and tell me who currently has access to it."*

The assistant calls ``set_mcp_enabled`` with ``{"name": "crm", "enabled":
false}``, then ``list_access_policies`` and reports the policies whose grants
name ``crm``. From that moment every data-plane call to ``crm`` is refused
with ``403 mcp_disabled``, and its tools vanish from every agent's catalogue;
the registration and policies stay intact for when it is switched back on.

Safety
------

- **This is host access.** ``register_mcp`` with ``transport = "stdio"``
  starts a process on the gateway host. Only grant ``admin_auth`` to people
  (and agents) you would give a shell on that machine.
- **Keep a human in the loop** for destructive operations. Most MCP clients
  can require confirmation per tool call; require it at least for
  ``register_mcp``, ``unregister_mcp`` and the ``delete_*`` tools.
- **Keep admin tokens out of everyday agents.** Agents doing ordinary work
  should hold tokens without the claims ``admin_auth`` requires, so a
  compromised work session cannot reconfigure the gateway. Give the
  operating assistant its own, separately issued administrator token.

See also
--------

- :doc:`/reference/admin-mcp` — the tool reference.
- :doc:`/architecture/decisions/0011-mcp-control-server` — the design.
