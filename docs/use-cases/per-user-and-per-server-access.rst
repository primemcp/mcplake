Per-User and Per-Server Access
==============================

The problem
-----------

You run several MCP servers — source control, tickets, a CRM, a production
database — and different people need different subsets. Some grants follow
teams; a few are exceptions for individuals: a contractor who needs one tool
for a month, an incident commander who needs database access now.

The approach
------------

Write one access policy per **capability**, matched on the claim that
expresses it. Team capabilities match a group claim; individual exceptions
match a unique claim such as ``sub`` or ``email``. Because policies are
additive, a person gets the union of every policy that matches them.

Configuration
-------------

.. code-block:: toml

   # Every engineer: read-only source control and tickets.
   [[access_policies]]
   name = "engineering-baseline"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^engineering$"
   [[access_policies.grants]]
   mcp = "git"
   tools = ["search_code", "get_file", "list_pull_requests"]
   [[access_policies.grants]]
   mcp = "tickets"
   tools = ["search", "get_ticket"]

   # Sales: the CRM, all of it.
   [[access_policies]]
   name = "sales-crm"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^sales$"
   [[access_policies.grants]]
   mcp = "crm"
   tools = ["*"]

   # One contractor, one tool. Literal strings ('...') keep the regex readable.
   [[access_policies]]
   name = "contractor-jdoe-tickets"
   [[access_policies.match]]
   path = "$.email"
   pattern = '^jdoe@contractor\.example\.com$'
   [[access_policies.grants]]
   mcp = "tickets"
   tools = ["get_ticket"]

   # Whoever holds the incident-commander role, while they hold it.
   [[access_policies]]
   name = "incident-commander-db"
   [[access_policies.match]]
   path = "$.roles[*]"
   pattern = "^incident-commander$"
   [[access_policies.grants]]
   mcp = "postgres-ro"
   tools = ["*"]

Individual grants at runtime
----------------------------

Exceptions for individuals are exactly the kind of change you want to make and
undo without a deployment. Create them through the admin API and delete them
when they expire:

.. code-block:: bash

   curl -X POST https://gateway.internal:8081/admin/access-policies \
     -H "Authorization: Bearer $ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"name": "contractor-jdoe-tickets",
          "match": [{"path": "$.email", "pattern": "^jdoe@contractor\\.example\\.com$"}],
          "grants": [{"mcp": "tickets", "tools": ["get_ticket"]}]}'

   # ...a month later
   curl -X DELETE https://gateway.internal:8081/admin/access-policies/contractor-jdoe-tickets \
     -H "Authorization: Bearer $ADMIN_TOKEN"

The grant applies to the contractor's next call and disappears on the next
call after the delete — including in an MCP session that is already open. To
suspend a grant without losing it, set its ``enabled`` flag to ``false``
instead.

Guidelines
----------

- **Prefer claims your identity provider manages.** A group or role granted
  and revoked in the provider needs no gateway change at all: the incident
  commander policy above follows the role automatically.
- **Match stable identifiers.** ``sub`` never changes; an email address can.
  Use whichever your provider guarantees to be unique.
- **Escape regular-expression metacharacters.** In ``^jdoe@contractor\.example\.com$``
  the dots are escaped; unescaped, they match any character. In TOML, use a
  literal string (single quotes) so the backslash is not a string escape.
- **Keep a small number of wildcard policies.** ``mcp = "*"`` also grants
  every MCP server registered in the future; reserve it for administrators.

See also
--------

- :doc:`/concepts/access-policies`, :doc:`/concepts/claim-rules`
- :doc:`/reference/admin-api`
