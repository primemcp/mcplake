Configuration and State
=======================

The gateway has two sources of truth, each for its own kind of setting. Mixing
them up is the most common surprise when operating it.

.. list-table::
   :header-rows: 1
   :widths: 22 39 39

   * -
     - The configuration file
     - The database
   * - Holds
     - ``[server]``, ``[oidc]``, ``[admin_auth]``, ``[admin_mcp]``, ``[mcp]``,
       ``[persistence]``
     - MCP registrations, access policies, filter policies
   * - Read
     - At every start
     - Continuously; changes are live on the next request
   * - Change it by
     - Editing the file and restarting
     - The admin API, the admin web UI or the MCP control server

Seeding: the file starts the database, once
-------------------------------------------

``[[mcps]]``, ``[[access_policies]]`` and ``[[filter_policies]]`` in the file are
**seed data**. On start, each entry is written to the database only if that
name has **never** been seeded before. After that, the database owns it:

- editing the entry in the file and restarting changes nothing — the gateway
  logs a ``WARN`` that the entry "DIFFERS from the stored record" and keeps
  the stored version;
- deleting an entry through the admin API is permanent — a restart does not
  bring it back from the file;
- adding an entry with a **new** name to the file seeds it on the next start.

This is deliberate: a revocation made through the admin API must not be
silently undone by a restart
(:doc:`ADR-0016 </architecture/decisions/0016-config-seeding-happens-once-per-entry>`).

To change a seeded entry, change it at runtime. To make the file win again —
in a test environment, say — delete the stored entry (or the database) and
restart.

.. tip::

   A workable pattern for production: keep the file minimal (servers,
   identity, persistence, and at most a bootstrap admin policy), and manage
   MCP registrations and policies at runtime — for example from a script or
   pipeline calling the admin API, which then *is* your source of truth under
   version control.

Where the database lives
------------------------

``[persistence]`` selects embedded SQLite (the default: one file, no external
service) or PostgreSQL, for when you want the state on a managed, backed-up
database.

.. note::

   Each gateway instance keeps registrations and policies in an in-memory
   cache and refreshes it when a change is made *through that instance*.
   Several instances on one database do not yet notify each other, so run a
   single instance, or route all administration through one and restart the
   others after changes.

.. code-block:: toml

   [persistence]
   driver = "postgres"
   dsn = "postgres://mcplake:secret@db.internal:5432/mcplake?sslmode=require"

Enabled flags
-------------

MCP registrations, access policies and filter policies each have an
``enabled`` flag (default ``true``), for switching something off without
deleting it:

.. list-table::
   :header-rows: 1
   :widths: 25 75

   * - Disabled…
     - Means
   * - MCP registration
     - Stays connected; every call is refused with ``403 mcp_disabled``; its
       tools leave the MCP catalogue.
   * - Access policy
     - Grants nothing.
   * - Filter policy
     - Removes nothing; other enabled filters still apply.

Flip them through the admin API or UI; see :doc:`/features/enable-disable`.

Reference
---------

- :doc:`/reference/configuration` — every configuration key.
- :ref:`reference-configuration-3-persistence` — persistence options.
- :doc:`/architecture/decisions/0006-gorm-sqlite-postgres-persistence`.
