Response Filtering
==================

Access policies decide whether a caller may use a tool at all. **Filter
policies** decide what the caller gets to see of its answer: they remove named
fields from a tool's result before it leaves the gateway.

This is what lets one ``get_employee`` tool serve both HR, who need salaries,
and everyone else, who need a phone number — without two MCP servers, two
tools, or trusting the agent to ignore what it was given.

.. code-block:: toml

   [[filter_policies]]
   name = "staff-hide-pay"
   mcp = "employee-directory"
   tool = "get_employee"
   drop_fields = ["$.salary_usd", "$.ssn_last4"]
   [[filter_policies.match]]
   path = "$.role"
   pattern = "^staff$"

A filter policy names **one** tool — ``mcp`` and ``tool`` are exact, with no
wildcards — the :doc:`claim rules <claim-rules>` it applies to, and the fields
to drop.

How filtering is applied
------------------------

After a call succeeds, the gateway collects every enabled filter policy for
that ``(mcp, tool)`` whose ``match`` holds for the caller, and removes the
**union** of their ``drop_fields``. Several policies can therefore apply to
one caller; each removes its own fields.

.. list-table::
   :header-rows: 1
   :widths: 30 35 35

   * - The tool returns
     - Caller with ``role=staff``
     - Caller with ``role=hr``
   * - ``{"id": 2, "name": "Marcus Webb", "salary_usd": 198000, "ssn_last4": "0193"}``
     - ``{"id": 2, "name": "Marcus Webb"}``
     - unchanged — no policy matches

Paths point into the tool's own result
--------------------------------------

``drop_fields`` are JSONPath expressions written against **the tool's
result as the MCP server returns it** — not against the MCP protocol envelope
around it. An MCP result can carry the same data more than once: as
``structuredContent``, and as JSON text inside ``content`` blocks. The gateway
applies every path to every copy, so a field cannot leak through the one you
did not think of.

.. code-block:: toml

   drop_fields = [
     "$.salary_usd",                  # a top-level field
     "$.address.street",              # a nested field
     "$.employees[*].salary_usd",     # a field in every element of a list
   ]

A path that selects nothing in a particular response is not an error — an
optional field may simply be absent. But when a policy applied to a call and
removed nothing at all, the gateway logs a ``WARN``: it almost always means the
tool's response shape changed and the path is now dead.

.. tip::

   Some MCP frameworks return a bare list in two shapes — wrapped as
   ``{"result": [...]}`` in ``structuredContent`` but unwrapped in the text
   copy. One path cannot reach both. Have list-returning tools return an
   object (``{"employees": [...]}``) so every copy has the same shape; see
   the note in :doc:`/getting-started/demo`.

Filtering fails closed
----------------------

If a filter applies to a call but a text block in the response is not JSON —
free-form prose, say — the fields cannot be located, so the gateway **refuses
to return the response** rather than pass it through unchecked:

- ``POST /v1/call`` answers ``502 filter_unenforceable``;
- MCP ``tools/call`` returns a result with ``isError: true`` carrying the same
  code.

A tool whose output is free text keeps working for callers no filter applies
to. Filter it only if it returns structured data.

What filtering is not
---------------------

Filtering is data minimization on the response, not access control. It does
not stop a caller from *invoking* a tool or choosing its arguments — use
access policies for that. And it filters by field, not by record: to hide
whole rows, give the caller a different tool or MCP server that only returns
what they may see.

Reference
---------

- :ref:`reference-configuration-6-filter-policies` — the configuration keys.
- :doc:`/architecture/decisions/0015-filter-the-tool-payload-not-the-transport-envelope`
  — why paths target the payload, and the fail-closed rule.
- :doc:`/architecture/decisions/0004-unified-policy-engine-for-access-and-filtering`
  — one rule engine for access and filtering.
