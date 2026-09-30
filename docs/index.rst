mcplake
=======

**An identity-aware gateway for the Model Context Protocol.**

|project| sits between AI agents and the MCP servers they use. Every caller
authenticates with a JWT from your OIDC provider; the gateway decides from the
token's claims which MCP servers and tools that caller may reach, and strips
the fields from each tool response that the caller is not allowed to see.

Agents connect to it as they would to any MCP server — over Streamable HTTP or
SSE — and see a tool catalogue filtered to exactly what their token allows.

.. mermaid::

   flowchart LR
       A["AI agent<br/>(MCP client)"] -- "JWT" --> G
       subgraph G["mcplake gateway"]
           direction TB
           V["Validate token"] --> P["Authorize (mcp, tool)<br/>from claims"]
           P --> C["Call downstream"] --> F["Filter response<br/>fields by claims"]
       end
       G --> M1["MCP: database"]
       G --> M2["MCP: filesystem"]
       G --> M3["MCP: internal API"]

.. grid:: 1 2 2 3
   :gutter: 3

   .. grid-item-card:: Claims-based access
      :link: concepts/access-policies
      :link-type: doc

      Grant MCP servers and individual tools to callers by any JWT claim,
      matched with JSONPath and regular expressions.

   .. grid-item-card:: Field-level filtering
      :link: concepts/response-filtering
      :link-type: doc

      Remove salary, SSN or any other field from a tool's response for callers
      who should not see it — fail-closed when it cannot.

   .. grid-item-card:: MCP in, MCP out
      :link: concepts/mcp-servers
      :link-type: doc

      Reach downstream servers over stdio, Streamable HTTP or SSE; serve every
      caller one namespaced, per-caller tool catalogue.

   .. grid-item-card:: Live administration
      :link: reference/admin-api
      :link-type: doc

      Register MCPs and change policies at runtime through a REST API, an admin
      web UI, or the gateway's own MCP control server.

   .. grid-item-card:: Self-healing connections
      :link: features/mcp-health-check
      :link-type: doc

      Downstream servers that restart or start late are reconnected
      automatically.

   .. grid-item-card:: Air-gap friendly
      :link: concepts/how-it-works
      :link-type: doc

      One Go binary, embedded SQLite by default, no telemetry and no outbound
      calls beyond your OIDC provider's keys.

Where to start
--------------

- **See it working in five minutes:** :doc:`getting-started/quickstart`
  runs the whole stack — identity provider, gateway, MCP servers — with one
  command.
- **Put it in front of your own MCP server:** :doc:`getting-started/first-gateway`.
- **Solve a specific problem:** the :doc:`use cases <use-cases/index>` — read-only
  vs read-write access, redacting fields, connecting clients, air-gapped
  deployment and more.
- **Understand the model:** :doc:`concepts/how-it-works`, then
  :doc:`concepts/claim-rules`.
- **Operate it:** the :doc:`how-to guides <how-to/index>` — identity provider
  setup, securing the control plane, production deployment, troubleshooting.
- **Look something up:** the :doc:`configuration reference
  <reference/configuration>` and the :doc:`APIs <reference/index>`.

.. toctree::
   :caption: Getting Started
   :maxdepth: 2
   :hidden:

   getting-started/index

.. toctree::
   :caption: Concepts
   :maxdepth: 2
   :hidden:

   concepts/index

.. toctree::
   :caption: Use Cases
   :maxdepth: 2
   :hidden:

   use-cases/index

.. toctree::
   :caption: How-To Guides
   :maxdepth: 2
   :hidden:

   how-to/index

.. toctree::
   :caption: Features
   :maxdepth: 2
   :hidden:

   features/index

.. toctree::
   :caption: Reference
   :maxdepth: 2
   :hidden:

   reference/index

.. toctree::
   :caption: Architecture
   :maxdepth: 2
   :hidden:

   architecture/index

.. toctree::
   :caption: Development
   :maxdepth: 2
   :hidden:

   development/index
