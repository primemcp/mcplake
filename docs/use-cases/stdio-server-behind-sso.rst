Share a Local stdio Server Behind SSO
=====================================

The problem
-----------

Many MCP servers only speak stdio: each developer runs their own copy on their
laptop, with their own copy of the API key or database password it needs.
Credentials are spread across machines, there is no record of who can use
what, and revoking access means rotating the shared secret.

The approach
------------

Run the server **once, on the gateway host**, as a ``stdio`` registration. The
secret lives only in the gateway's configuration and is handed only to that
subprocess. Developers connect to the gateway over HTTP with their own SSO
tokens, and access policies decide who gets which tools.

.. mermaid::

   flowchart LR
       subgraph before["Before: every laptop"]
           L1["laptop A<br/>server + API key"]
           L2["laptop B<br/>server + API key"]
       end
       subgraph after["After: one gateway"]
           D1["laptop A"] -- "SSO token" --> GW["mcplake"]
           D2["laptop B"] -- "SSO token" --> GW
           GW -- "stdio" --> S["server<br/>+ API key"]
       end

Configuration
-------------

.. code-block:: toml

   [[mcps]]
   name = "vendor-api"
   type = "stdio"
   command = "/opt/mcp/vendor-api-mcp"
   arguments = ["--region", "eu"]
   env = { VENDOR_API_KEY = "sk-live-..." }

   [[access_policies]]
   name = "vendor-api-users"
   [[access_policies.match]]
   path = "$.groups[*]"
   pattern = "^(support|solutions)$"
   [[access_policies.grants]]
   mcp = "vendor-api"
   tools = ["*"]

Developers replace the local server entry in their MCP client with the gateway
(see :doc:`connect-mcp-clients`); the tools keep their names, prefixed with
``vendor-api__``.

What changes
------------

- **One copy of the secret.** It is in the gateway's configuration (or its
  database, if registered through the admin API) and in the environment of one
  process. Rotating it is one change.
- **Access follows identity.** Joining or leaving the ``support`` group in
  your identity provider grants or revokes the tools. No secret rotation is
  needed when someone leaves.
- **The subprocess is isolated from the gateway's environment.** It receives
  only ``PATH``, ``HOME``, ``LANG``, ``LC_ALL``, ``TZ``, ``TMPDIR`` and its own
  ``env`` — not the gateway's other secrets. If it needs another variable,
  declare it in ``env``.
- **Fields can be filtered** per group if the server returns more than some
  users should see; see :doc:`redact-sensitive-fields`.

Things to check
---------------

- **Per-user state.** A stdio server started once is shared by all callers. If
  it keeps per-session state, or acts with the identity of whoever started it
  (for example a personal access token), everyone now acts as that identity.
  Use a service account with the narrowest rights that cover everyone's needs,
  or register separate instances per group.
- **Host dependencies.** The server's runtime (Python, Node, …) must be
  installed on the gateway host, or in the gateway's container image.
- **Protect the configuration.** The config file and the database now hold
  the secret; restrict their permissions accordingly, and remember that the
  admin API can read registrations — including their ``env`` — so it must be
  behind ``admin_auth``.

See also
--------

- :doc:`/concepts/mcp-servers` — transports and the subprocess environment.
- :doc:`/architecture/decisions/0018-stdio-mcp-subprocesses-get-a-minimal-base-environment`
