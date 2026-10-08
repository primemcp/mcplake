Testing
=======

Go
--

.. code-block:: bash

   make test          # all modules, verbose, with coverage
   make test-race     # the same under the race detector

   go test ./router/...                        # one module
   go test ./router/... -run TestClaimRule     # one test

Tests need no external services: persistence tests use SQLite, and
end-to-end tests in ``cmd/gateway/app`` start real downstream MCP servers by
re-executing the test binary as a fixture process. They exercise the whole
pipeline — authentication, policies, the MCP client and filtering — in one
process.

Shared fixtures live in ``testdata/``. ``testdata/claim_stringify_cases.json``
is read by both the Go claim-rule tests and the admin UI's TypeScript tests,
so the two implementations of claim matching cannot drift apart.

Admin UI
--------

.. code-block:: bash

   make ui-test       # Vitest, once

See :doc:`admin-ui` for watch mode and linting.

API spec
--------

If you change a control-plane handler or its annotations, regenerate the
OpenAPI spec and commit it with the change:

.. code-block:: bash

   make swagger
   make swagger-check   # fails if the committed spec is stale (the CI check from ADR-0007)

The documentation renders the committed spec, so the published API reference
follows automatically.

Documentation
-------------

.. code-block:: bash

   make docs-check    # strict build (warnings are errors) and link check

Before pushing
--------------

.. code-block:: bash

   make check         # fmt, vet, test, build
   make ui-test       # if you touched the UI
   make docs-check    # if you touched docs/
