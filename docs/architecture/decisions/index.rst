Architecture Decision Records
=============================

Each significant technical decision is recorded as an ADR: the context that
forced it, what was decided, the alternatives, and the consequences. An ADR is
not edited after it is accepted; a later decision supersedes it instead.

New records go in this directory as ``NNNN-short-title.rst``, numbered
sequentially.

.. note::

   Two records share the number 0010. Both are kept as written; they are
   distinguished by title.

.. toctree::
   :maxdepth: 1

   0001-use-fasthttp-for-gateway-server
   0002-jsonpath-regexp-claim-rule-engine
   0003-dynamic-mcp-registration-and-schema-discovery
   0004-unified-policy-engine-for-access-and-filtering
   0005-use-gin-for-control-plane-api
   0006-gorm-sqlite-postgres-persistence
   0007-swaggo-for-control-plane-api-docs
   0008-frontend-only-multi-tool-filter-grouping
   0009-operator-enable-disable-flag
   0010-control-plane-admin-authentication
   0010-filter-reuse-is-clone-not-link
   0011-mcp-control-server
   0012-toml-configuration-format
   0013-periodic-mcp-schema-refresh
   0014-admin-ui-oidc-pkce-login
   0015-filter-the-tool-payload-not-the-transport-envelope
   0016-config-seeding-happens-once-per-entry
   0017-http-and-sse-transports-for-downstream-mcps
   0018-stdio-mcp-subprocesses-get-a-minimal-base-environment
   0019-reconnect-downstream-mcps-on-a-health-check-loop
   0020-serve-the-admin-ui-from-its-own-container
   0021-serve-mcp-on-the-data-plane
   0022-sphinx-and-restructuredtext-for-project-documentation
