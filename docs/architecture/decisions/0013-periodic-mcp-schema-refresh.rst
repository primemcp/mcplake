ADR-0013: Periodic MCP Schema Refresh
=====================================

:Status: Accepted
:Date: 2026-09-10

Context
-------

:doc:`ADR-0003 </architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery>` fetches a
downstream MCP's tool list exactly once, at registration
(``Registry.Register`` → ``client.ListTools``), and caches it in
``cache.Registry``. A long-lived gateway then serves a frozen view of every MCP:

- A **tool added** to a running MCP is invisible to the gateway until an
  operator re-registers that MCP (``DELETE`` + ``POST /admin/mcps``, or a restart).
  ``HasTool`` returns false, so ``POST /v1/call`` for the new tool is
  ``404 tool_not_found`` even though the MCP would serve it.
- A **tool removed** from a running MCP still shows as callable — the stale
  cache entry passes ``HasTool``, the call is routed, and it fails at the MCP
  with ``502 upstream_error`` instead of the cleaner ``404 tool_not_found``.
- A tool whose **input/output schema changed** keeps serving the old schema to
  the response filter and any schema consumers.

Constraints and context:

- MCP does define a server→client ``notifications/tools/list_changed``, but
  relying on it requires every downstream MCP to implement and emit it
  correctly; the gateway needs a mechanism that works regardless.
- A refresh must not be disruptive: re-``Register`` reconnects (for stdio, spawns
  a fresh subprocess), which can reset MCP-side session state and adds process
  churn every interval.
- A transient ``tools/list`` failure (MCP briefly busy) must not evict a working
  MCP from rotation.
- The gateway is single-instance by default; there is no external scheduler to
  lean on.
- Operators should be able to turn this off (it is extra downstream traffic)
  and today's behaviour — refresh only at registration — must remain the
  default so nothing changes on upgrade.

Decision
--------

Add an optional **gateway-side polling refresh** of MCP tool schemas.

- **Config.** A new ``[mcp]`` table with ``schema_refresh_interval``, a Go duration
  string. Omitted or ``"0"`` (the default) disables it — schemas are fetched only
  at registration, exactly as today. A negative value is rejected by
  ``Config.Validate()``.
- **What a refresh does.** For each registration that is ``StatusActive`` with a
  live ``Client``, call ``client.ListTools(ctx)`` on the **existing** client — no
  reconnect, no subprocess spawn — and atomically swap the cached ``Tools`` map
  in via the Registry's existing single-write ``set`` path, preserving ``Status``,
  ``Client``, ``Enabled``, ``Connect``, and ``Transport``. A concurrent
  ``Get``/``Resolve``/``HasTool`` sees either the complete old map or the complete
  new one.
- **Failure handling.** A ``tools/list`` error is logged at ``WARN`` and the
  registration is left untouched — previous ``Tools`` and ``Status`` intact. A blip
  never knocks an MCP out; the next tick retries. The swap also re-checks the
  registration still exists and still holds the same client (an ``Unregister`` or
  re-``Register`` may have raced) and skips it if not.
- **Scheduling.** One ``cache.SchemaRefresher`` runs a ``time.Ticker`` loop calling
  ``Registry.RefreshActive(ctx)`` each tick, started in ``app.Run`` alongside the
  data-plane and control-plane servers, its lifetime bound to the app context.
  When the interval is zero it is never constructed.
- **Disabled MCPs.** A disabled MCP is still ``StatusActive`` (ADR-0009 keeps it
  connected), so it *is* refreshed — its cached schema stays current for the
  moment it is re-enabled. This is cheap and avoids a stale-on-re-enable
  surprise.

New API on ``cache.Registry``:

- ``RefreshTools(ctx, name) error`` — refresh one MCP (no-op for an
  absent / non-active / clientless name; error only on a ``ListTools`` failure).
- ``RefreshActive(ctx) []RefreshResult`` — refresh every active MCP, collecting
  per-MCP outcomes; one failure never stops the rest.

Alternatives Considered
-----------------------

Alternative A: Re-``Register`` (reconnect + rediscover) each interval
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Also recovers a dead MCP (new client, fresh subprocess).
- Reuses the fully-tested ``Register`` make-before-break path.

Disadvantages:

- Spawns a new stdio subprocess every interval per MCP — process churn, and it
  can reset MCP-side session/connection state that a long-running server holds.
- Conflates two concerns: "is the MCP still reachable" (health) and "is our
  schema current" (freshness). Reconnect-on-failure belongs to a separate
  health/reconnect mechanism, not the schema refresh.

Kept as future work: a health check that re-``Register``\ s an MCP that has gone
``unreachable``, independent of this interval.

Alternative B: Honour ``notifications/tools/list_changed`` from the MCP
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Event-driven; zero polling traffic; refreshes exactly when needed.

Disadvantages:

- Requires every downstream MCP to implement and correctly emit the
  notification; many will not. Cannot be the only mechanism.
- The gateway's MCP client would need to subscribe and route notifications into
  the Registry — more moving parts than a ticker.

Complementary, not a substitute: if added later, it can call the same
``Registry.RefreshTools``.

Alternative C: Per-MCP interval overrides
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- A fast-changing MCP could refresh more often than a static one.

Disadvantages:

- More config surface for a Phase-1 feature with no demonstrated need. One
  global interval is enough; per-MCP can be added later without breaking the
  global key.

Alternative D: On-demand only — a ``POST /admin/mcps/:name/refresh`` endpoint
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- No background traffic; operator controls exactly when.

Disadvantages:

- Doesn't solve the "gateway drifts while nobody is watching" case, which is
  the actual problem. Worth adding alongside (it can call the same Registry
  method), but not instead.

Decision Criteria
-----------------

- **Non-disruptive** — no reconnect, no subprocess churn on the happy path.
- **Fail-safe** — a transient error never removes a working MCP.
- **Opt-in, zero-change default** — off unless an interval is configured.
- **Minimal surface** — one Registry method pair + one ticker, no new module,
  no new listener, one config key.
- **Concurrency-correct** — reuses the Registry's existing atomic swap.

Rationale
---------

The problem is staleness, not liveness, so the fix is to re-read the tool list
on the connection we already hold and swap it in atomically — the cheapest
thing that closes the gap. Reconnecting (Alternative A) solves a different
problem and costs more every interval. Event-driven refresh (Alternative B) is
strictly better when it works but can't be relied on across arbitrary MCPs, so
polling is the floor. Keeping it off by default means upgrade is a no-op and
operators opt in with one duration.

Consequences
------------

Positive
~~~~~~~~

- A tool added or removed downstream is reflected within ~one interval, with no
  operator action and no reconnect.
- A removed tool returns the clean ``404 tool_not_found`` sooner instead of a
  downstream ``502``.
- The response filter and any schema consumers see current input/output
  schemas.
- Reuses the Registry's atomic ``set``; no new concurrency primitives.

Negative
~~~~~~~~

- Extra ``tools/list`` traffic: one round-trip per active MCP per interval.
  Bounded by the operator-chosen interval; off by default.
- Bounded staleness remains (up to one interval); this is a polling design, not
  push.
- A tool that is added and removed again within one interval is never observed —
  acceptable for a schema cache.

Risks
~~~~~

- Too short an interval against many MCPs adds noticeable load. Mitigated by the
  default being disabled and by docs recommending minutes, not seconds.
- A refresh that races an ``Unregister`` must not resurrect the entry. Handled by
  the re-check-before-swap in ``RefreshTools``.

Follow-up
~~~~~~~~~

- [STRIKEOUT:A health/reconnect mechanism that re-``Register``\ s an MCP stuck in
  ``unreachable`` (Alternative A's genuine use).] Done in
  :doc:`ADR-0019 </architecture/decisions/0019-reconnect-downstream-mcps-on-a-health-check-loop>`, as a
  separate loop rather than an extension of this one: it probes with ``ping``
  rather than ``tools/list``, and it is on by default where this is opt-in.
- An on-demand ``POST /admin/mcps/:name/refresh`` (Alternative D) and/or a
  ``refresh_mcp`` MCP control tool, both calling ``Registry.RefreshTools``.
- Consuming ``notifications/tools/list_changed`` where an MCP provides it
  (Alternative B).

Validation
----------

- ``cache``: ``RefreshTools`` swaps only ``Tools``; added tool appears, removed tool
  disappears; a ``ListTools`` error leaves the previous map and ``Status``; a race
  with ``Get`` is clean under ``-race``; a refresh after ``Unregister`` is a no-op.
- ``cache``: ``SchemaRefresher`` ticks at the interval, calls ``RefreshActive`` each
  tick, logs failures without stopping, and returns on context cancel.
- ``config``: ``[mcp] schema_refresh_interval`` present / absent / ``"0"`` / negative.
- ``cmd/gateway/app``: a configured interval starts the loop and shutdown stops it
  cleanly; an end-to-end test where a running MCP's tool set changes and the
  data plane picks it up within an interval.

References
----------

- :doc:`ADR-0003 </architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery>` — the
  once-at-registration discovery this ADR follows up.
- :doc:`ADR-0009 </architecture/decisions/0009-operator-enable-disable-flag>` — why a disabled MCP is still
  ``StatusActive`` and therefore still refreshed.
- :doc:`ADR-0012 </architecture/decisions/0012-toml-configuration-format>` — the ``[mcp]`` table's format.
- :repo:`CONFIG.md <docs/CONFIG.md>` — the ``mcp.schema_refresh_interval`` key.
