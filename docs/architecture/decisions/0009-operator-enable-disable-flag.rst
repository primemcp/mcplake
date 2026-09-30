ADR-0009: Operator ``Enabled`` Flag with Per-Object Semantics
=============================================================

:Status: Accepted
:Date: 2026-09-08

Context
-------

Operators need to pause a single MCP, temporarily revoke a user's access, or
turn a response filter off for debugging, without losing the record. Before
this change the only lever was ``DELETE``, which drops the row entirely — and for
an MCP also discards the discovered tool schema cache and forces a full
re-register (reconnect + re-discover) to undo. The admin webui (Task #79)
already shipped an enable/disable toggle in its UI with no backend behind it.

The three runtime-managed objects — ``cache.MCPRegistration``,
``router.AccessPolicy``, ``router.FilterPolicy`` — sit at different points in the
:ref:`request pipeline <architecture-overview-pipeline-per-request>`, so "disabled" cannot
mean one uniform thing.

Decision
--------

Add a single boolean ``Enabled`` (default ``true``) to all three objects, persisted
(GORM column, ``NOT NULL DEFAULT true``), seedable from ``config.toml`` (``enabled``
key), and editable via the admin API. Enforcement differs per object, matching
where each already acts:

- **MCP registration** — the data-plane pipeline checks
  ``Registry.Disabled(mcp)`` *after* authorization and *before* ``Resolve``, and
  returns a new ``403 mcp_disabled``, distinct from ``404 mcp_not_found``. The
  registration, its cached schemas, and its live client are all retained;
  re-enabling needs no reconnect.
- **Access policy** — ``Engine.Authorize`` skips a disabled policy before
  evaluating its ``Match``. It grants nothing; a caller authorized only by it
  gets ``403 forbidden``.
- **Filter policy** — ``Engine.FieldsToRemove`` skips a disabled policy. Its
  ``drop_fields`` do not contribute; other enabled policies for the same
  ``(mcp, tool)`` still apply.

Boundary representations use ``*bool`` (nil ⇒ enabled); the in-memory domain
types keep a plain ``bool``.

Alternatives Considered
-----------------------

A uniform "disabled ⇒ treat as absent" rule for all three
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Simpler to describe, but wrong for the MCP case: an operator disabling an MCP
wants a clear, distinct signal (``mcp_disabled``), not a ``404`` that looks
identical to a typo in the MCP name or an MCP that was never registered.
Collapsing the two also loses the "still connected, re-enable is instant"
property — the point of disable over delete.

``Disabled bool`` on the domain types instead of ``Enabled bool``
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The zero value of ``Disabled`` is "active", which is safer against a forgotten
struct field. Rejected for naming consistency: the persisted column, the config
key, the API field, and the webui toggle all speak ``enabled`` (default true).
Mitigated instead by setting ``Enabled`` explicitly at every production
construction site and in tests, and by the ``*bool`` boundary types below.

Plain ``bool`` at the config/DB/API boundaries
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

A plain ``bool`` at a decode boundary cannot distinguish an omitted key from
``enabled = false``, and GORM treats a zero-valued ``bool`` field with a ``default:``
tag as "unset" and substitutes the default on write — so an explicit ``false``
could never be persisted. ``*bool`` at each boundary (nil ⇒ default true) avoids
both.

Consequences
------------

Positive
~~~~~~~~

- Reversible kill switch for any single object, no data loss, MCP re-enable
  with no reconnect.
- Zero behaviour change on upgrade: an omitted config key, a pre-existing DB
  row (backfilled by ``AutoMigrate``), and an admin request without the field all
  mean enabled.
- New ``403 mcp_disabled`` is unambiguous in logs and to clients.

Negative
~~~~~~~~

- ``Enabled bool``'s zero value is "disabled", so an in-process struct literal
  that forgets the field is silently inert. Contained by setting it explicitly
  everywhere and by boundary ``*bool``\ s; a ``Disabled`` field would not have this
  property.
- Three semantics to learn rather than one. Documented in
  :repo:`features/enable-disable.md <docs/features/enable-disable.md>`, the pipeline
  diagram, and :ref:`architecture-data-enabledisable-gates`.

Follow-up
~~~~~~~~~

- The webui toggles (Epic #76) wire to ``PATCH /admin/mcps/:name`` and the policy
  ``enabled`` field.
- No audit trail of who toggled what / when — folds into the broader audit-log
  work (Phase 3), not this change.

Validation
----------

- ``router``: ``Authorize`` / ``FieldsToRemove`` skip disabled policies (incl. an
  enabled sibling still applying).
- ``cache``: ``Registry.Disabled`` / ``SetEnabled``; ``Register`` carrying a
  seeded-disabled state through connect+activate.
- ``gateway/internal``: ``403 mcp_disabled`` distinct from ``404``, no downstream
  call.
- ``persistence``: ``Enabled`` round-trip; ``AutoMigrate`` backfill of a legacy DB.
- ``config``: ``enabled`` present/absent/false parsing and converter propagation.
- ``cmd/gateway/app``: ``TestApp_EnableDisable_EndToEnd`` — config-seeded disabled
  entries of all three kinds through the full running gateway (real JWKS, real
  SQLite, real MCP subprocess): disabled MCP ⇒ ``403 mcp_disabled``, disabled
  access policy ⇒ ``403 forbidden``, disabled filter policy ⇒ field not stripped
  while an enabled filter's field is.

References
----------

- :repo:`features/enable-disable.md <docs/features/enable-disable.md>`
- :doc:`/reference/configuration` — the ``enabled`` key.
- :doc:`/reference/admin-api` — ``enabled`` field and ``PATCH /admin/mcps/:name``.
- :doc:`ADR-0004 </architecture/decisions/0004-unified-policy-engine-for-access-and-filtering>` — the two
  policy lists this flag gates.
