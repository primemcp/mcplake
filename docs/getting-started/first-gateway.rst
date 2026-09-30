Your First Gateway
==================

This tutorial puts mcplake in front of an MCP server, using your own OIDC
provider. By the end you will have:

- one MCP server registered with the gateway,
- two groups of callers with different access, told apart by a JWT claim,
- a field-level filter that hides pay data from one of them,
- and a way to change all of that at runtime.

It uses the small ``employee-directory`` MCP server from the repository's demo
(:repo:`deploy/demo/mcp-servers/employee_directory.py`) because its responses
have fields worth filtering. Any MCP server works the same way.

Before you start
----------------

- A built gateway — see :doc:`installation`.
- Python 3.10+ to run the example MCP server.
- An OIDC provider where you can add a custom claim to access tokens. This
  tutorial uses a claim called ``role`` with the values ``hr`` and ``staff``;
  use whatever your provider already emits (a group list works too — see
  :doc:`/concepts/claim-rules`).

Collect three values from your provider:

.. list-table::
   :header-rows: 1
   :widths: 25 75

   * - Value
     - Where to find it
   * - JWKS URL
     - The ``jwks_uri`` in ``https://<issuer>/.well-known/openid-configuration``.
   * - Issuer
     - The ``iss`` claim of its tokens.
   * - Audience
     - The ``aud`` claim of the tokens your agents will present — usually the
       client or API identifier you registered for the gateway.

Step 1 — Prepare the MCP server
-------------------------------

.. code-block:: bash

   python3 -m venv ~/mcplake-demo-mcp
   ~/mcplake-demo-mcp/bin/pip install -r deploy/demo/mcp-servers/requirements.txt

The gateway will start this server itself, as a subprocess speaking MCP over
stdio. It exposes three tools: ``list_employees``, ``get_employee`` and
``list_departments``.

Step 2 — Write the configuration
--------------------------------

Save this as ``config.toml``, replacing the ``[oidc]`` values and the two
paths:

.. code-block:: toml

   [server]
   data_plane_addr = "127.0.0.1:8080"      # agents connect here
   control_plane_addr = "127.0.0.1:8081"   # admin API and UI; keep it private

   [oidc]
   jwks_url = "https://auth.example.com/.well-known/jwks.json"
   issuer = "https://auth.example.com"
   audience = "mcp-gateway"

   [persistence]
   driver = "sqlite"
   dsn = "gateway.db"

   [[mcps]]
   name = "employee-directory"
   type = "stdio"
   command = "/home/you/mcplake-demo-mcp/bin/python"
   arguments = ["/path/to/mcplake/deploy/demo/mcp-servers/employee_directory.py"]

   # HR sees everything the directory offers.
   [[access_policies]]
   name = "hr-full-access"
   [[access_policies.match]]
   path = "$.role"
   pattern = "^hr$"
   [[access_policies.grants]]
   mcp = "employee-directory"
   tools = ["*"]

   # Staff may look people up, and nothing else.
   [[access_policies]]
   name = "staff-lookup"
   [[access_policies.match]]
   path = "$.role"
   pattern = "^staff$"
   [[access_policies.grants]]
   mcp = "employee-directory"
   tools = ["get_employee", "list_employees"]

   # ...and never sees pay data, in either tool.
   [[filter_policies]]
   name = "staff-hide-pay-get-employee"
   mcp = "employee-directory"
   tool = "get_employee"
   drop_fields = ["$.salary_usd", "$.ssn_last4"]
   [[filter_policies.match]]
   path = "$.role"
   pattern = "^staff$"

   [[filter_policies]]
   name = "staff-hide-pay-list-employees"
   mcp = "employee-directory"
   tool = "list_employees"
   drop_fields = ["$.employees[*].salary_usd", "$.employees[*].ssn_last4"]
   [[filter_policies.match]]
   path = "$.role"
   pattern = "^staff$"

What this says:

- **Access policies** decide *whether* a call is allowed. A call is allowed when
  any policy whose ``match`` rules all hold for the caller's token grants the
  requested ``(mcp, tool)`` pair. Anything not granted is refused.
- **Filter policies** decide *what the caller sees* of an allowed call. The
  ``drop_fields`` are JSONPath expressions into the tool's own result.
- Patterns are regular expressions and are **not anchored** for you: write
  ``^staff$``, not ``staff``, unless you mean "contains".

Step 3 — Start the gateway
--------------------------

.. code-block:: bash

   mcp-gateway --config config.toml

The log shows the listeners, and a warning you will fix in step 7:

.. code-block:: text

   WARN control-plane admin API is UNAUTHENTICATED — set admin_auth.match in config, ...
   INFO mcplake gateway starting data_plane_addr=127.0.0.1:8080 control_plane_addr=127.0.0.1:8081

Confirm the MCP server was started and its tools discovered:

.. code-block:: bash

   curl -s http://127.0.0.1:8081/admin/mcps | jq '.[] | {name, status, tools: (.tools | keys)}'

.. code-block:: json

   {
     "name": "employee-directory",
     "status": "active",
     "tools": ["get_employee", "list_departments", "list_employees"]
   }

Step 4 — Call it as HR and as staff
-----------------------------------

Get two access tokens from your provider — one for a user whose ``role`` claim
is ``hr``, one for ``staff`` — and export them as ``HR_TOKEN`` and
``STAFF_TOKEN``. Then:

.. code-block:: bash

   call() {
     curl -s -X POST http://127.0.0.1:8080/v1/call \
       -H "Authorization: Bearer $1" -H "Content-Type: application/json" -d "$2"
   }

   call "$HR_TOKEN"    '{"mcp":"employee-directory","tool":"get_employee","arguments":{"employee_id":2}}' | jq .structuredContent
   call "$STAFF_TOKEN" '{"mcp":"employee-directory","tool":"get_employee","arguments":{"employee_id":2}}' | jq .structuredContent

HR gets the whole record; staff gets it without ``salary_usd`` and
``ssn_last4``:

.. code-block:: json

   {"department": "Engineering", "email": "marcus.webb@example.com", "id": 2,
    "name": "Marcus Webb", "title": "Engineering Manager"}

Now a tool staff was not granted:

.. code-block:: bash

   call "$STAFF_TOKEN" '{"mcp":"employee-directory","tool":"list_departments"}'

.. code-block:: json

   {"error": "forbidden", "message": "not authorized to call this tool"}

with status ``403``. A token with no matching policy at all gets the same, and
a request without a token gets ``401``.

.. tip::

   If every call returns ``401 unauthorized``, decode one of your tokens (for
   example at a local JWT debugger) and compare its ``iss`` and ``aud`` with
   ``[oidc]`` — they must match exactly.

Step 5 — Connect an MCP client
------------------------------

The same policies apply when an agent connects over MCP. Configure your MCP
client with the data plane's endpoint and a token:

.. code-block:: javascript

   {
     "type": "http",
     "url": "http://127.0.0.1:8080/v1/mcp",
     "headers": { "Authorization": "Bearer <STAFF_TOKEN>" }
   }

The staff client's tool list is ``employee-directory__get_employee`` and
``employee-directory__list_employees`` — ``list_departments`` is not offered at
all, because the catalogue is the access policy made visible. An HR client
sees all three.

Step 6 — Change a policy at runtime
-----------------------------------

Policies live in the gateway's database. ``config.toml`` seeded them on the
first start; from now on, change them through the admin API (or the admin
web UI) — no restart:

.. code-block:: bash

   curl -s -X PUT http://127.0.0.1:8081/admin/filter-policies/staff-hide-pay-get-employee \
     -H "Content-Type: application/json" \
     -d '{"name": "staff-hide-pay-get-employee",
          "mcp": "employee-directory", "tool": "get_employee",
          "drop_fields": ["$.salary_usd", "$.ssn_last4", "$.email"],
          "match": [{"path": "$.role", "pattern": "^staff$"}]}'

Staff's next ``get_employee`` call comes back without ``email`` too.

.. important::

   Editing ``config.toml`` and restarting does **not** change an entry that
   was already seeded. The gateway logs a ``WARN`` naming the entry
   ("already seeded and DIFFERS from the stored record") and keeps the stored
   version. See :doc:`/concepts/configuration-and-state`.

Step 7 — Lock down the control plane
------------------------------------

The admin API can register MCP servers — for ``stdio``, that means starting a
process on the gateway host — so it must not be open. Add rules that an
administrator's token must satisfy:

.. code-block:: toml

   [[admin_auth.match]]
   path = "$.role"
   pattern = "^admin$"

Restart the gateway (``admin_auth`` is read from the file on every start). The
warning is gone, and every ``/admin/*`` request now needs
``Authorization: Bearer <token>`` whose claims satisfy those rules. To let the
admin web UI sign operators in by itself, configure
:ref:`admin_auth.login <reference-configuration-admin-ui-sign-in>`.

Where next
----------

- :doc:`/concepts/index` — the model behind what you just configured.
- :doc:`/reference/configuration` — every option.
- :doc:`/reference/admin-api` — everything the control plane can do.
