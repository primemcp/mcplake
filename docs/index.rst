mcplake
=======

|project| is an open-source gateway for the `Model Context Protocol
<https://modelcontextprotocol.io>`_ (MCP). It sits between AI agents and the
MCP servers they use, authenticates every caller with OIDC, decides from the
caller's JWT claims which MCPs and tools they may reach, and strips fields from
tool responses the caller is not allowed to see.

.. toctree::
   :caption: Getting Started
   :maxdepth: 2

   getting-started/index

.. toctree::
   :caption: Features
   :maxdepth: 2

   features/index

.. toctree::
   :caption: Reference
   :maxdepth: 2

   reference/index

.. toctree::
   :caption: Architecture
   :maxdepth: 2

   architecture/index
