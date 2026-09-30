Upgrade and Back Up
===================

What to back up
---------------

.. list-table::
   :header-rows: 1
   :widths: 30 70

   * - Item
     - Why
   * - The configuration file
     - Listeners, identity provider, persistence and admin settings — and the
       seed entries.
   * - The database
     - The source of truth for MCP registrations and policies once seeded.
       With SQLite, the file named by ``persistence.dsn``; with PostgreSQL,
       your usual database backups.

Both contain secrets if any MCP registration has ``env`` values or the DSN
has a password; protect the backups accordingly.

Backing up SQLite
~~~~~~~~~~~~~~~~~

Copy the file while the gateway is stopped, or use SQLite's online backup
while it runs:

.. code-block:: bash

   sqlite3 /var/lib/mcplake/gateway.db ".backup '/backup/gateway-$(date +%F).db'"

Exporting as JSON
~~~~~~~~~~~~~~~~~

The admin API gives a readable export that is also a restore path (see
:ref:`how-to-use-postgresql-moving-from-sqlite`):

.. code-block:: bash

   for r in mcps access-policies filter-policies; do
     curl -s "$ADMIN/$r" -H "Authorization: Bearer $ADMIN_TOKEN" > "mcplake-$r.json"
   done

Upgrading the gateway
---------------------

1. Read the changes between your version and the new one, including any new
   :doc:`decision records </architecture/decisions/index>`.
2. Back up the database.
3. Replace the binary (or image) and restart.

The gateway creates missing tables and columns on startup. To roll back,
restore the backup taken in step 2 along with the previous binary.

After an upgrade, check the log for warnings, in particular
``config entry already seeded and DIFFERS from the stored record``: it names
entries whose definition in your configuration file no longer matches the
database. The database version is the one in force.

Applying configuration-file changes to seeded entries
-----------------------------------------------------

Editing an MCP server or policy in ``config.toml`` does not change an entry
that was already seeded (:doc:`/concepts/configuration-and-state`). To apply
it, do one of:

- make the same change through the admin API or UI (recommended);
- delete the entry through the admin API — the next restart does **not**
  re-seed it, so re-create it through the API as well;
- in a disposable environment, start with an empty database.

Resetting the demo
------------------

See :ref:`getting-started-demo-upgrading-or-resetting-the-demo`.
