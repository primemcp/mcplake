ADR-0016: Config Seeding Happens Once Per Entry
===============================================

:Status: Accepted
:Date: 2026-09-22

Context
-------

:doc:`ADR-0006 </architecture/decisions/0006-gorm-sqlite-postgres-persistence>` made the database the
durable home of MCP registrations and access/filter policies, and cast the
config file as a *seed*. Four documents state the resulting contract plainly —
``docs/CONFIG.md``: "the database becomes the source of truth from then on; the
config file is a seed mechanism, not a parallel state store", and the same in
``docs/features/enable-disable.md``, ``docs/architecture/components.md`` and
ADR-0006 itself ("eliminating the two-sources-of-truth risk").

The code did something else. ``App.New`` called ``Config.Seed`` unconditionally on
every boot, and each repository's ``OnConflict`` force-updated the
security-relevant columns — ``enabled``, ``match``, ``grants``, ``drop_fields``. Because
an omitted ``enabled`` key resolves to *true*, a restart re-applied the config file
over whatever the operator had since done through the admin API.

The 2026-09-22 security audit traced it end to end. An operator disables a
leaking MCP with ``PATCH /admin/mcps/:name {"enabled": false}``; the data plane
starts returning ``403 mcp_disabled``; the pod is then restarted for an unrelated
reason; ``Seed`` writes ``enabled = true`` back and ``RegisterAll`` brings the MCP up
serving again. No log line, no operator action. The same mechanism resurrected a
deleted access policy and reverted a ``drop_fields`` list an operator had tightened
during an incident — that last one re-opening a data leak.

Requirements for the fix:

- **A revocation must survive a restart.** Disable and delete are both
  revocations; a control that silently reverts to permissive is worse than no
  control, because the operator believes it is in force.
- **Both halves.** Overwrite (disable reverted) and resurrection (delete undone)
  are the same bug seen from two sides, and insert-only semantics only fix the
  first: after a delete the name is absent, so an insert-only seed recreates it.
- **Adding a new entry to the config file must still work.** Operators do
  bootstrap new MCPs and policies this way, and silently ignoring a new
  ``[[mcps]]`` block would trade one confusing behaviour for another.
- No new operational step, no manual migration, no flag an operator must
  remember.

Decision
--------

**Each named config entry is seeded exactly once, ever.**

A ``seed_markers`` table records ``(kind, name)`` for every entry that has been
seeded — ``kind`` being one of ``mcp``, ``access_policy``, ``filter_policy``. ``Seed``
loads the whole set in one query, backfills a marker for any row that
already exists in the store but has none — a store from before this table
existed — and then, for each entry in the config file:

- if a marker exists, skip it and log at ``DEBUG`` that the stored record wins;
- otherwise write the entry and then record the marker.

The backfill step is what makes adopting this fix itself safe: without it,
the very first boot after upgrading would have zero markers and would
overwrite every existing row from config exactly once — reverting a
disable an operator made specifically to prepare for the upgrade. It only
covers rows that still exist, so it cannot recover a policy that was
deleted before the marker table existed; see Consequences.

The marker is written *after* the entry, so a crash in between costs one
harmless re-seed of that entry rather than a lost entry or a failed startup.
``Mark`` is idempotent for the same reason.

The marker deliberately outlives the row it describes. That is the whole reason
it is a separate table rather than a column: after ``DELETE /admin/access-policies/:name``
there is no row left to carry the information, and the marker is the only thing
standing between a deleted policy and its recreation on the next boot.

Consequences of "once per entry" as the unit:

- A config entry added after the first boot has no marker, so it is seeded. The
  bootstrap workflow keeps working.
- Editing an entry that has already been seeded has no effect. The stored record
  is authoritative — which is what the documentation always said.

Alternatives Considered
-----------------------

Alternative A: Keep upserting, but log every overwrite
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Smallest diff; no schema change.
- The operator can, in principle, see what happened.

Disadvantages:

- Does not fix anything. The control is still silently reverted; the operator
  only gets a chance to notice afterwards, in a log they may not read, after the
  MCP has already served traffic again.
- Leaves the documentation wrong.

Alternative B: Insert-only seeding (``OnConflict DoNothing``)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- No new table; a one-line change per repository.
- Fixes the overwrite half completely.

Disadvantages:

- Does not fix resurrection: a deleted entry's name is absent, so the next boot
  inserts it again. Half a fix for a security control is not a fix.
- Requires a separate insert-only repository method anyway, since the admin API
  legitimately needs the upsert path.

Alternative C: Seed once, globally (a single "store has been seeded" marker)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Simplest possible marker — one row, no per-entry bookkeeping.
- Fixes both halves.

Disadvantages:

- An entry added to the config file after the first boot is silently ignored
  forever, which is a new and equally confusing footgun.
- Makes the config file useless for anything but the very first start, which is
  a bigger behaviour change than the bug warrants.

Alternative D: Config always wins; document it and warn at startup
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Preserves the current behaviour for GitOps-style deployments where the file is
  intended to be authoritative.
- No schema change.

Disadvantages:

- Requires rewriting four documents to say the opposite of what they say now,
  and makes runtime admin writes second-class: an operator's emergency disable
  would be valid only until the next restart.
- Two sources of truth is exactly what ADR-0006 set out to eliminate.
- If config-is-authoritative is ever wanted, it should be an explicit mode, not
  the accidental consequence of an upsert clause.

Decision Criteria
-----------------

1. Does a runtime revocation survive a restart? (A and B: no.)
2. Are both revocation shapes — disable and delete — covered? (B: no.)
3. Does bootstrapping a new entry from config still work? (C: no.)
4. Does the code end up matching the documented contract? (D: no, it rewrites
   the contract instead.)
5. Operational cost: no new step, no manual migration.

Only the chosen design satisfies all five.

Rationale
---------

The bug was a mismatch between a documented contract and the clause that
implemented it. The cheapest correct fix is the one that makes the contract
literally true, and "the config file seeds an entry once; the store owns it
afterwards" is both the documented promise and the smallest rule that covers
delete as well as disable.

Paying for a table is worth it because the information genuinely does not exist
anywhere else: a deleted row cannot remember that it was once seeded. Any design
that avoids the table has to give up either resurrection-safety (B) or the
add-an-entry workflow (C).

Consequences
------------

Positive
~~~~~~~~

- Disable, delete and a tightened ``drop_fields`` all survive a restart. The
  enable/disable kill switch (:doc:`ADR-0009 </architecture/decisions/0009-operator-enable-disable-flag>`)
  is now actually durable.
- The database really is the source of truth, as four documents already claimed.
- Adding an MCP or policy to the config file still works.

Negative
~~~~~~~~

- **Editing an already-seeded entry in the config file no longer takes effect.**
  This is a behaviour change. Operators who edited config and restarted to change
  a policy must now use the admin API (or delete the stored entry first). ``Seed``
  logs each skipped entry at ``DEBUG``, and ``CONFIG.md`` says so.
- One more table and one more repository. ``AutoMigrate`` creates it; no manual
  migration step.
- An existing deployment upgrading to this version has no markers yet, so
  ``Seed`` backfills one for every row that already exists before seeding
  anything (see ``backfillSeedMarkers``) — a name whose row is still there
  (an MCP the operator disabled, a ``drop_fields`` list they tightened) is
  preserved exactly as it stood, not reverted. The one case backfill
  cannot cover is a policy the operator deleted *before* upgrading: with
  no row and no marker, it is indistinguishable from a brand-new config
  entry and is seeded fresh once, on that first post-upgrade boot only.
  Operators who deleted something through the admin API before upgrading
  should re-delete it (or confirm it stayed gone) after that one boot.

Risks
~~~~~

- A store restored from a backup taken before an entry was seeded will seed it
  again. That is the correct reading of the marker, but it means marker and data
  must be backed up together — they are in the same database, so ordinary
  backups already satisfy this.
- Renaming one of the ``SeedKind*`` constants would orphan existing markers and
  cause a one-time re-seed of that kind. They are stored verbatim and must be
  treated as a wire format.

Follow-up
~~~~~~~~~

- If a config-is-authoritative mode is ever wanted (Alternative D), add it as an
  explicit ``seed_mode = "always"`` rather than by loosening this default.
- Surface "config entry skipped because it is already seeded" in the admin UI,
  so the operator sees it without reading logs. **Partly addressed in #179**,
  in the logs rather than the UI: a skip whose stored record *disagrees* with
  the config entry now logs at ``WARN`` naming the fields that differ, instead of
  at ``DEBUG`` — which, being below the default level, meant the operator was in
  practice told nothing at all. Field names only, never values: an MCP's
  ``arguments`` commonly carry a credential, and a warning on every boot is the
  wrong place for one. The seeding semantics decided above are unchanged; only
  the reporting is. The admin-UI half remains open.

Validation
----------

- ``persistence``: marker round-trip, idempotent ``Mark``, ``kind`` participating in
  the key, and a marker outliving the row it describes.
- ``config``: a re-seed does not re-apply a changed entry; an entry added later is
  still seeded; a runtime disable, a runtime delete and a tightened
  ``drop_fields`` all survive a re-seed; a store with pre-existing, marker-less
  rows (the exact upgrade scenario) backfills correctly — a disabled MCP
  stays disabled across the upgrade boot, and a policy deleted before
  upgrading is seeded fresh exactly once, then stays gone on every boot
  after that. On the reporting added in #179: a changed entry warns and names
  the differing field while still not being applied; an unchanged entry, an
  MCP that has merely since connected (its ``Status``/``Tools`` are the gateway's,
  not the config's), an omitted ``arguments`` list round-tripping as ``[]``, and an
  entry the operator deleted all stay quiet; and no field value reaches the log.
- ``cmd/gateway/app``: the whole gateway started twice against one store — disable
  an MCP and delete a policy through the admin API, restart, and both stay
  revoked; a config entry added between the two boots is applied.

References
----------

- :doc:`ADR-0006 </architecture/decisions/0006-gorm-sqlite-postgres-persistence>` — made the database
  durable and named the config file a seed; this ADR makes that literal.
- :doc:`ADR-0009 </architecture/decisions/0009-operator-enable-disable-flag>` — the kill switch whose
  durability this restores.
- :repo:`CONFIG.md <docs/CONFIG.md>` — the operator-facing contract.
- :repo:`features/enable-disable.md <docs/features/enable-disable.md>`
