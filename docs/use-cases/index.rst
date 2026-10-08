Use Cases
=========

Worked solutions to problems people put mcplake in front of their MCP servers
to solve. Each one starts from the problem, gives a complete configuration,
and explains how the pieces fit. They assume you have read
:doc:`/getting-started/first-gateway`.

.. grid:: 1 1 2 2
   :gutter: 3

   .. grid-item-card:: Read-only and read-write access to one database
      :link: read-only-and-read-write
      :link-type: doc

      Analysts query, engineers change — through the same MCP server,
      decided by their identity.

   .. grid-item-card:: Redact sensitive fields
      :link: redact-sensitive-fields
      :link-type: doc

      Return salaries, SSNs or contact details only to the people who need
      them.

   .. grid-item-card:: Per-user and per-server access
      :link: per-user-and-per-server-access
      :link-type: doc

      Grant individual people or teams specific MCP servers and tools.

   .. grid-item-card:: Connect MCP clients to the gateway
      :link: connect-mcp-clients
      :link-type: doc

      Point Claude Code, VS Code or your own agent at one endpoint instead of
      many servers.

   .. grid-item-card:: Let an agent operate the gateway
      :link: agent-operated-gateway
      :link-type: doc

      Register servers and change policies from an MCP client, through the
      gateway's own control server.

   .. grid-item-card:: Share a local stdio server behind SSO
      :link: stdio-server-behind-sso
      :link-type: doc

      Turn a server every developer runs locally, with their own copy of the
      credentials, into one shared, access-controlled service.

   .. grid-item-card:: Air-gapped deployment
      :link: air-gapped-deployment
      :link-type: doc

      Run the gateway in a network with no internet access.

.. toctree::
   :hidden:

   read-only-and-read-write
   redact-sensitive-fields
   per-user-and-per-server-access
   connect-mcp-clients
   agent-operated-gateway
   stdio-server-behind-sso
   air-gapped-deployment
