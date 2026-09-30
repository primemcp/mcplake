Repository and Workspace
========================

mcplake is a Go **multi-module workspace**: each component is its own Go
module with its own ``go.mod``, tied together for development by ``go.work``.

Layout
------

.. list-table::
   :header-rows: 1
   :widths: 28 72

   * - Directory
     - Contents
   * - ``auth/``
     - JWT validation against the OIDC provider's JWKS.
   * - ``router/``
     - The claim-rule engine: claim rules, access and filter policies.
   * - ``filter/``
     - Removing ``drop_fields`` from tool results.
   * - ``mcp/``
     - The MCP client for downstream servers (stdio, Streamable HTTP, SSE).
   * - ``cache/``
     - The MCP registry: registrations, tool schemas, health checks and
       schema refresh.
   * - ``config/``
     - Loading and validating the TOML configuration; seeding the database.
   * - ``persistence/``
     - GORM models and repositories (SQLite, PostgreSQL).
   * - ``gateway/``
     - The data plane (``gateway/internal``) and the control plane
       (``gateway/internal/controlplane``), including the admin MCP server and
       the admin web UI sources (``controlplane/webui``, a Bun/React project).
   * - ``cmd/gateway/``
     - The ``mcp-gateway`` entry point and application wiring (``app/``).
   * - ``cmd/webui/``
     - The standalone admin UI server.
   * - ``deploy/``
     - Container builds for the demo and the admin UI; the demo's Keycloak
       realm, database seed and MCP servers.
   * - ``testdata/``
     - Fixtures shared across modules and with the web UI's tests.
   * - ``docs/``
     - This documentation (a Python/Sphinx project, not a Go module).

Dependencies point inward: ``router``, ``filter`` and ``auth`` know nothing of
HTTP servers or storage; ``gateway`` and ``cmd/gateway/app`` wire them
together. The architecture pages describe the boundaries:
:doc:`/architecture/components`.

Working with modules
--------------------

.. code-block:: bash

   go work sync               # after changing a module's dependencies
   go work use ./newmodule    # add a new module to the workspace, then: make tidy

   # Test one module on its own, as its consumers would build it
   cd auth && GOWORK=off go test ./...

Make targets
------------

.. list-table::
   :header-rows: 1
   :widths: 28 72

   * - Target
     - Does
   * - ``make build``
     - Build the admin UI, then ``cmd/gateway/mcp-gateway`` with the UI
       embedded.
   * - ``make test`` / ``make test-race``
     - Run every module's tests (with the race detector).
   * - ``make fmt`` / ``make vet`` / ``make lint``
     - ``gofmt`` and ``go vet`` across all modules.
   * - ``make check``
     - ``fmt``, ``vet``, ``test`` and ``build`` together — run before pushing.
   * - ``make tidy`` / ``make sync``
     - ``go work sync``.
   * - ``make swagger`` / ``make swagger-check``
     - Regenerate the control-plane OpenAPI spec from handler annotations /
       fail if the committed spec is stale. Needs ``swag``
       (``go install github.com/swaggo/swag/cmd/swag@latest``).
   * - ``make ui-dev`` / ``make ui-build`` / ``make ui-test``
     - Admin UI dev server, production build and tests — see
       :doc:`admin-ui`.
   * - ``make docs`` / ``make docs-serve`` / ``make docs-check``
     - This documentation — see :doc:`documentation`.
   * - ``make clean``
     - Remove build output and the Go build cache.

Dev container
-------------

``.devcontainer/`` defines a Fedora-based development container with Go,
Bun, ``uv`` and the GitHub CLI — the complete toolchain for the gateway, the
UI and the docs.

Coding guidelines
-----------------

- Target the Go version in ``go.work`` and use the modern standard library
  (``slices``, ``maps``, ``errors.Is``/``errors.As``, ``log/slog``).
- Tests use ``testify`` for assertions; write the test with the change.
- Significant technical decisions get an ADR under
  ``docs/architecture/decisions/`` — see :doc:`/architecture/decisions/index`.
