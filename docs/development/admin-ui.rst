Admin Web UI Development
========================

The admin UI is a React and TypeScript single-page app built with Vite and
managed with `Bun <https://bun.sh>`_. Its sources are in
``gateway/internal/controlplane/webui/``, next to the Go file that embeds the
built output (``go:embed`` cannot reach outside its module), and it talks to
the gateway only through the control-plane API.

Run it with live reload
-----------------------

Start a gateway with a local configuration, then the Vite dev server:

.. code-block:: bash

   ./cmd/gateway/mcp-gateway --config config.toml   # control plane on :8081
   make ui-dev

The dev server proxies ``/admin`` to ``http://localhost:8081``, so the UI uses
your local gateway's API. Set ``control_plane_addr`` to port ``8081`` or change
the proxy target in ``vite.config.ts``.

Build and test
--------------

.. code-block:: bash

   make ui-build      # production build into webui/dist, embedded by the next go build
   make ui-test       # Vitest

   cd gateway/internal/controlplane/webui
   bun run test:watch # tests in watch mode
   bun run lint       # oxlint

``webui/dist`` is build output and is not committed. A Go build of the gateway
needs it to exist; see :doc:`/getting-started/installation`.

Two ways the UI is served
-------------------------

- **Embedded** in the gateway binary, on the control-plane listener.
- **Standalone**, by the ``webui`` server (``cmd/webui``) in its own container,
  which serves the same build and proxies ``/admin`` to the gateway
  (:doc:`ADR-0020 </architecture/decisions/0020-serve-the-admin-ui-from-its-own-container>`).

The UI's behaviour is documented in :doc:`/features/admin-webui`.
