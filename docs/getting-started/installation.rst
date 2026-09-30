Installation
============

mcplake is built from source. The result is a single static binary,
``mcp-gateway``, with the admin web UI embedded in it.

Requirements
------------

.. list-table::
   :header-rows: 1
   :widths: 25 75

   * - Tool
     - Needed for
   * - Go 1.27.1 or later
     - Building the gateway.
   * - `Bun <https://bun.sh>`_ 1.x
     - Building the admin web UI that is embedded in the binary.
   * - An OIDC provider
     - Issuing the JWTs callers present (Keycloak, Auth0, Okta, Entra ID,
       Dex, … — anything that publishes a JWKS).

The runtime has no other dependencies: persistence defaults to an embedded,
pure-Go SQLite, and the binary is built with ``CGO_ENABLED=0``-compatible
dependencies.

Build
-----

.. code-block:: bash

   git clone https://github.com/atsokha/mcplake.git
   cd mcplake
   make build

``make build`` builds the admin UI with Bun and then the gateway, producing
``cmd/gateway/mcp-gateway``. Copy that file wherever you like.

.. note::

   A bare ``go build ./cmd/gateway`` on a fresh clone fails with
   ``pattern all:webui/dist: no matching files found``: the admin UI's build
   output is embedded into the binary and is not committed. Build the UI
   first (``make ui-build``), or — if you do not want the embedded UI, for
   example because you serve it from its own container — create an empty
   placeholder:

   .. code-block:: bash

      mkdir -p gateway/internal/controlplane/webui/dist
      echo '<!doctype html><title>mcplake</title>' > gateway/internal/controlplane/webui/dist/index.html
      CGO_ENABLED=0 go build -o mcp-gateway ./cmd/gateway

Run
---

.. code-block:: bash

   ./cmd/gateway/mcp-gateway --config config.toml

The gateway reads everything from the one TOML file; there are no environment
variable overrides. Start from :repo:`config.example.toml` or follow
:doc:`first-gateway`. On startup it logs the two listeners it opened:

.. code-block:: text

   INFO mcplake gateway starting data_plane_addr=127.0.0.1:8080 control_plane_addr=127.0.0.1:8081

The admin web UI as its own container
-------------------------------------

The UI is also shipped as a separate static server, ``webui``, that serves the
built app and reverse-proxies ``/admin/*`` to the gateway's control plane
(:doc:`ADR-0020 </architecture/decisions/0020-serve-the-admin-ui-from-its-own-container>`).
Build it with :repo:`deploy/webui/Dockerfile` from the repository root:

.. code-block:: bash

   docker build -f deploy/webui/Dockerfile -t mcplake-webui .
   docker run -p 8082:8081 -e WEBUI_API_TARGET=http://gateway-host:8081 mcplake-webui

Its flags and environment variables are listed in
:ref:`reference-cli-webui`.

Container image for the gateway
-------------------------------

The repository does not publish a production gateway image yet.
:repo:`deploy/demo/Dockerfile` shows the build — UI stage, Go stage, runtime
stage — but its runtime stage is a Python image because the demo runs a Python
MCP server as a subprocess. For your own image, reuse its first two stages and
choose a runtime base that contains whatever your ``stdio`` MCP servers need to
execute.

Verify the build
----------------

.. code-block:: bash

   make test     # all Go modules
   make ui-test  # admin UI (Vitest)
