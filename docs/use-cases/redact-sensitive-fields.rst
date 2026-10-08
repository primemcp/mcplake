Redact Sensitive Fields
=======================

The problem
-----------

An MCP server returns records containing personal or confidential data — pay,
national ID numbers, home addresses. Several groups need the tool, but not
all of them may see every field. Changing the server to know about your
organization's roles is not an option, and asking the agent not to repeat a
field is not a control.

The approach
------------

Grant the tool to everyone who needs it, then use **filter policies** to strip
fields per group. Each filter names one tool, the claims it applies to, and
JSONPath expressions for the fields to drop. When several filters match a
caller, all of their fields are removed.

Configuration
-------------

A directory server with ``get_employee`` and ``list_employees`` tools, used by
three groups:

.. list-table::
   :header-rows: 1

   * - Group
     - Sees
   * - ``hr``
     - everything
   * - ``managers``
     - everything except national ID and home address
   * - ``staff``
     - name, title, department, work email only

.. code-block:: toml

   [[access_policies]]
   name = "directory-users"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^(hr|managers|staff)$"
   [[access_policies.grants]]
   mcp = "employee-directory"
   tools = ["get_employee", "list_employees"]

   # Everyone except HR loses the most sensitive fields.
   [[filter_policies]]
   name = "non-hr-get-employee"
   mcp = "employee-directory"
   tool = "get_employee"
   drop_fields = ["$.ssn_last4", "$.home_address"]
   [[filter_policies.match]]
   path = "$.groups[*]"
   pattern = "^(managers|staff)$"

   # Staff additionally lose pay.
   [[filter_policies]]
   name = "staff-get-employee"
   mcp = "employee-directory"
   tool = "get_employee"
   drop_fields = ["$.salary_usd"]
   [[filter_policies.match]]
   path = "$.groups[*]"
   pattern = "^staff$"

   # The same rules for the list tool: paths go through the list.
   [[filter_policies]]
   name = "non-hr-list-employees"
   mcp = "employee-directory"
   tool = "list_employees"
   drop_fields = ["$.employees[*].ssn_last4", "$.employees[*].home_address"]
   [[filter_policies.match]]
   path = "$.groups[*]"
   pattern = "^(managers|staff)$"

   [[filter_policies]]
   name = "staff-list-employees"
   mcp = "employee-directory"
   tool = "list_employees"
   drop_fields = ["$.employees[*].salary_usd"]
   [[filter_policies.match]]
   path = "$.groups[*]"
   pattern = "^staff$"

A member of ``staff`` matches two filters on ``get_employee`` and loses all
three fields; a manager matches one and loses two; HR matches none.

.. tip::

   Structure filters by *what is hidden from whom*, as above, rather than one
   filter per group listing everything that group loses. Adding a field that
   only HR may see is then a one-line change to one policy.

Getting the paths right
-----------------------

- Paths are written against the **tool's result**, as the server returns it
  — call the tool once as an unfiltered user and look at the shape.
- Use ``[*]`` to reach into every element of a list:
  ``$.employees[*].salary_usd``; nested objects work too:
  ``$.employees[*].address.street``.
- A path that matches nothing is silently fine for a single response, but if
  a whole policy removes nothing from a response it applied to, the gateway
  logs a ``WARN``. Watch for it after the MCP server is upgraded — a renamed
  field would otherwise pass through unfiltered.
- The gateway applies each path to every copy of the result — the structured
  content and JSON text blocks — so a field cannot leak through a duplicate.

When the response is not JSON
-----------------------------

If a filter applies to a call but the tool answers with free text, the
gateway cannot find the fields and refuses the response
(``502 filter_unenforceable``) rather than pass it through. Filter only tools
that return structured data; for a free-text tool, restrict *access* instead.

See also
--------

- :doc:`/concepts/response-filtering` — the rules in full.
- :doc:`/getting-started/quickstart` — this in action in the demo.
