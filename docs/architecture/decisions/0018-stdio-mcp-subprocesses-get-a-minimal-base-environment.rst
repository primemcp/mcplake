ADR-0018: stdio MCP Subprocesses Get a Minimal Base Environment, Not the Gateway's
==================================================================================

:Status: Accepted
:Date: 2026-09-23

Context
-------

``mcp.newTransport``'s stdio branch built the subprocess with ``exec.CommandContext``
and never set ``cmd.Env``:

.. code-block:: go

   return &sdk.CommandTransport{
       Command: exec.CommandContext(ctx, cfg.Command, cfg.Arguments...),
   }, cfg.Command, nil

Per ``os/exec``'s documented behavior, a nil ``Env`` means the child inherits the
*entire* environment of the process that spawned it — here, the gateway
itself. That gateway process routinely holds, in its own environment or a
config-derived value it never intended to hand out, the persistence DSN
(``[persistence].dsn``, commonly a Postgres connection string with credentials),
OIDC client configuration, and anything else an operator or the surrounding
deployment tooling set. Every ``stdio`` MCP the gateway registers — including
one a ``POST /admin/mcps`` caller supplies at runtime, not just a config-file
entry an operator wrote (see
:ref:`architecture-security-control-plane-access-is-host-code-execution`,
already host code execution by design) — received all of it.

This was tracked as a known gap (`#166 <https://github.com/atsokha/mcplake/issues/166>`__,
split out of the initial security audit that also produced ADR-0015 and
ADR-0016) rather than fixed alongside those, since closing it changes what
every existing ``stdio`` MCP receives — a behavior change worth its own review,
not a line item alongside unrelated findings.

Decision
--------

An MCP subprocess's environment is now built explicitly, never inherited:

1. A **documented base set** — ``PATH``, ``HOME``, ``LANG``, ``LC_ALL``, ``TZ``,
   ``TMPDIR`` — copied from the gateway's own environment when present. This is
   the minimum a subprocess needs to resolve its own tooling (``PATH``),
   locate caches/config it manages itself (``HOME``), behave predictably with
   text and time (``LANG``/``LC_ALL``/``TZ``), and write temp files somewhere that
   is actually writable (``TMPDIR``). None of these six is more sensitive than
   the fact that the gateway is running on this host at all.
2. The MCP's own declared ``Env`` (``mcp.Config.Env``, threaded through
   ``cache.ConnectConfig.Env``, ``config.MCPConfig.Env`` (``env`` in TOML), the
   ``connect.env`` REST field, and the ``register_mcp`` MCP tool's flat ``env``
   field), overlaid on top — an explicit key here always wins over the same
   key in the base set, since the operator named it on purpose.

Nothing else in the gateway's environment reaches the child. An MCP that
needs a credential gets it through ``env``, explicitly, the same way any other
connection detail (``command``, ``arguments``) is supplied — not implicitly by
running in the same process tree as something that happens to have it.

``http``/``sse`` transports are unaffected: there is no subprocess, so ``Env`` is
rejected on those entries the same way ``command`` already is
(:doc:`ADR-0017 </architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps>`).

Alternatives Considered
-----------------------

**A. Inherit everything, document the risk.** The status quo. Rejected: a
documented footgun is still a footgun, and the fix is not expensive enough to
justify leaving it.

B. Inherit an operator-configured allowlist of variable names, resolved
from the gateway's own environment at spawn time (e.g. ``env_from_gateway = ["HTTP_PROXY"]``), instead of a fixed base set plus explicit values.
Rejected for the default: it still couples an MCP's environment to whatever
the gateway happens to have, which is exactly the coupling this ADR removes.
Nothing rules out adding this as an opt-in escape hatch later if an operator
needs to pass through something dynamic (a proxy setting that varies by
deployment); today's ``Env`` map already covers every case the audit or the
demo config needed.

C. No base set at all — every subprocess gets exactly the operator's ``Env``,
nothing more. Rejected: this breaks the ordinary case (a ``stdio`` MCP that
shells out to a package manager or interpreter needs ``PATH`` to do it) for
every existing config, turning a security fix into a functional regression
operators would have to work around one variable at a time.

Decision Criteria
-----------------

- **Least privilege**: an MCP sees only what it needs to run and what it was
  explicitly given — not a byproduct of the gateway's own configuration.
- **No functional regression for the common case**: the base set is chosen
  so a typical ``stdio`` MCP (a binary or interpreter resolved via ``PATH``,
  writing temp files, behaving locale-appropriately) keeps working with zero
  config changes.
- **Explicit over implicit**: anything beyond the base set is named by the
  operator, in the same place the rest of the connection is configured.

Rationale
---------

The base set was chosen by asking what breaks *without* each variable, not by
copying a general-purpose shell's environment. ``PATH`` and ``HOME`` are the two
that matter most: without them, a ``stdio`` MCP that is itself a wrapper
(``bunx``, a ``pip``-installed console script, anything that shells out) commonly
fails outright, and the compose demo's own ``employee-directory`` MCP (a
``python3`` script — see ``deploy/demo/config.toml``) needs neither more than
this. ``LANG``/``LC_ALL``/``TZ``/``TMPDIR`` round out "keeps working exactly like it
did in the same locale/timezone/temp directory," at a cost of six variables
whose values are the sort of thing every process on the host already
observes, not a design detail specific to this gateway.

Consequences
------------

- **Breaking change**: an existing ``stdio`` MCP that relies on inheriting a
  gateway environment variable outside the base set (a credential, a proxy
  setting, anything config-derived) stops seeing it and must be given it
  through ``env`` instead. Documented in ``docs/reference/configuration.rst``'s ``env`` field.
- ``cache.ConnectConfig``, ``config.MCPConfig``, the REST DTOs, and the
  ``register_mcp`` MCP tool all gained an ``env`` field with no persistence
  schema change (GORM's ``datatypes.JSON`` columns round-trip a new struct
  field automatically — the same property ADR-0016 relied on).
- The admin web UI does not yet expose ``env`` in its add/edit form (out of
  this ADR's scope — ``EndpointDetail.tsx``'s save path now carries the
  endpoint's existing ``env`` through unedited, so registering via the REST
  API or the MCP control server and then editing via the UI does not
  silently wipe it).

Validation
----------

- ``mcp/client_test.go``: a gateway-only environment variable does not reach
  the subprocess; the documented base set (``PATH``) does; a declared ``Env``
  entry reaches the subprocess without opening the door to the rest of the
  gateway's environment; a declared value overrides the same key in the base
  set.
- ``cmd/gateway/app/schema_refresh_e2e_test.go``'s growing-MCP fixture, which
  used to rely on ``t.Setenv`` + inheritance to hand the subprocess a sentinel
  file path, now declares it through ``Env`` — a live demonstration of the
  migration every affected operator config needs to make.

References
----------

- `#166 <https://github.com/atsokha/mcplake/issues/166>`__ — the tracked finding
- :ref:`architecture-security-control-plane-access-is-host-code-execution` —
  control-plane access as host code execution
- :doc:`ADR-0015 </architecture/decisions/0015-filter-the-tool-payload-not-the-transport-envelope>`,
  :doc:`ADR-0016 </architecture/decisions/0016-config-seeding-happens-once-per-entry>` — the other
  audit findings from the same review
- :doc:`ADR-0017 </architecture/decisions/0017-http-and-sse-transports-for-downstream-mcps>` — why ``Env``
  only applies to ``stdio``
