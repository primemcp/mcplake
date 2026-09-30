Access Policies
===============

An access policy answers one question: **may this caller invoke this tool on
this MCP server?**

.. code-block:: toml

   [[access_policies]]
   name = "db-reader"
   [[access_policies.match]]        # who: claim rules, all must match
   path = "$.role"
   pattern = "^db-reader$"
   [[access_policies.grants]]       # what: (mcp, tools) pairs
   mcp = "postgres-ro"
   tools = ["*"]

A policy has a unique ``name``, a list of :doc:`claim rules <claim-rules>`
under ``match``, and one or more ``grants``.

How a call is decided
---------------------

A call to ``(mcp, tool)`` is **allowed** if at least one enabled policy

1. has ``match`` rules that all hold for the caller's claims, **and**
2. has a grant that covers the pair.

Otherwise it is refused with ``403 forbidden``. There are no deny rules:
policies only ever add access, and anything not granted is refused. To take
access away, remove or narrow the grant that gives it.

Grants
------

.. list-table::
   :header-rows: 1
   :widths: 35 65

   * - Grant
     - Covers
   * - ``mcp = "crm"``, ``tools = ["search", "get_account"]``
     - Exactly those two tools on ``crm``.
   * - ``mcp = "crm"``, ``tools = ["*"]``
     - Every tool ``crm`` advertises — including tools it adds later.
   * - ``mcp = "*"``, ``tools = ["search"]``
     - A tool named ``search`` on any registered MCP.
   * - ``mcp = "*"``, ``tools = ["*"]``
     - Everything, including MCP servers registered later. Reserve for
       administrators.

``*`` is the only wildcard, and it stands for a whole name: ``get_*`` is a
literal tool name, not a prefix match. Tool names are the MCP server's own
names (``get_user``), not the ``<mcp>__<tool>`` names MCP clients see.

Policies compose
----------------

Because every matching policy contributes its grants, it is natural to write
policies per *capability* and let claims pick which ones apply:

.. code-block:: toml

   # Everyone in engineering can read tickets.
   [[access_policies]]
   name = "tickets-read"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^engineering$"
   [[access_policies.grants]]
   mcp = "tickets"
   tools = ["search", "get_ticket"]

   # On-call engineers can also change them.
   [[access_policies]]
   name = "tickets-write-oncall"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^engineering$"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^oncall$"
   [[access_policies.grants]]
   mcp = "tickets"
   tools = ["update_ticket", "assign_ticket"]

An on-call engineer matches both policies and gets all four tools; everyone
else in engineering gets two.

What callers see
----------------

Access policies are enforced on every call, and over MCP they also shape the
catalogue: ``tools/list`` returns only the granted pairs. A caller with no
grants sees an empty tool list rather than an error.

Authorization is checked before the gateway looks the MCP or tool up, so a
``403`` never reveals whether something exists.

Changes are live
----------------

Policies are stored in the gateway's database and cached in memory. A change
made through the admin API, the admin web UI or the MCP control server applies
to the next request — including the next call in an MCP session that is
already open. Each policy also has an ``enabled`` switch: a disabled policy
grants nothing, which is a quick, reversible way to revoke a group's access.

Reference
---------

- :ref:`reference-configuration-5-access-policies` — the configuration keys.
- :doc:`/reference/admin-api` — managing policies at runtime.
- :doc:`/architecture/decisions/0004-unified-policy-engine-for-access-and-filtering`
  — the design.
