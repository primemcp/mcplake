Read-Only and Read-Write Access to One Database
===============================================

The problem
-----------

You want agents to work with a PostgreSQL database through an MCP server.
Analysts should be able to query it; only the platform team should be able to
change it. The MCP server itself has no notion of who is calling — whoever can
reach it can do whatever its database user can do.

The approach
------------

Register the **same MCP server twice**, as two registrations with different
database credentials and settings, and grant each registration to a different
group:

.. mermaid::

   flowchart LR
       An["Analyst<br/>groups: analysts"] --> G["mcplake"]
       Pl["Platform engineer<br/>groups: platform"] --> G
       G -- "analysts" --> RO["postgres-ro<br/>user: reporting_ro<br/>--access-mode=restricted"]
       G -- "platform" --> RW["postgres-rw<br/>user: app_admin<br/>--access-mode=unrestricted"]
       RO --> DB[("PostgreSQL")]
       RW --> DB

The database enforces the difference (the read-only user cannot write, whatever
the agent asks for), and the gateway decides who reaches which registration.
Neither depends on the agent behaving.

Configuration
-------------

This example uses `postgres-mcp <https://github.com/crystaldba/postgres-mcp>`_,
which reads its connection string from ``DATABASE_URI`` and has a restricted
(read-only) mode. Any database MCP server works the same way.

.. code-block:: toml

   [[mcps]]
   name = "postgres-ro"
   type = "stdio"
   command = "postgres-mcp"
   arguments = ["--access-mode=restricted"]
   env = { DATABASE_URI = "postgresql://reporting_ro:secret@db.internal:5432/app" }

   [[mcps]]
   name = "postgres-rw"
   type = "stdio"
   command = "postgres-mcp"
   arguments = ["--access-mode=unrestricted"]
   env = { DATABASE_URI = "postgresql://app_admin:secret@db.internal:5432/app" }

   [[access_policies]]
   name = "analysts-query"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^analysts$"
   [[access_policies.grants]]
   mcp = "postgres-ro"
   tools = ["*"]

   [[access_policies]]
   name = "platform-full"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^platform$"
   [[access_policies.grants]]
   mcp = "postgres-ro"
   tools = ["*"]
   [[access_policies.grants]]
   mcp = "postgres-rw"
   tools = ["*"]

What each group sees over MCP:

.. list-table::
   :header-rows: 1

   * - Caller
     - Tools offered
   * - ``groups: ["analysts"]``
     - ``postgres-ro__*``
   * - ``groups: ["platform"]``
     - ``postgres-ro__*`` and ``postgres-rw__*``
   * - anyone else
     - none

Notes
-----

- **Credentials stay on the gateway.** ``env`` values are given only to that
  subprocess; agents never see a connection string, and the gateway's own
  environment is not passed through. See :doc:`/concepts/mcp-servers`.
- **Keep the read-only guarantee in the database.** Give ``reporting_ro``
  only ``SELECT`` privileges. The MCP server's restricted mode and the
  gateway's policies are additional layers, not substitutes.
- **Narrow further with tool lists.** If the read-write registration exposes
  administrative tools you do not want even the platform team's agents to use,
  grant a list (``tools = ["execute_sql", "explain_query"]``) instead of
  ``"*"``.
- **Network servers work the same way.** If the MCP server runs elsewhere, use
  two ``type = "http"`` (or ``"sse"``) registrations pointing at two instances.

See also
--------

- :doc:`/concepts/access-policies`
- :doc:`redact-sensitive-fields` — to hide columns rather than whole tools.
