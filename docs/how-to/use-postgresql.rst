Use PostgreSQL for Persistence
==============================

By default the gateway keeps MCP registrations and policies in an embedded
SQLite file. Switch to PostgreSQL when you want that state on a managed,
backed-up database server.

1. Create a database and user
-----------------------------

.. code-block:: sql

   CREATE ROLE mcplake LOGIN PASSWORD 'change-me';
   CREATE DATABASE mcplake OWNER mcplake;

The gateway creates and updates its own tables on startup, so the user needs
to be able to create tables in its schema — owning the database, as above, is
the simplest way.

2. Point the gateway at it
--------------------------

.. code-block:: toml

   [persistence]
   driver = "postgres"
   dsn = "postgres://mcplake:change-me@db.internal:5432/mcplake?sslmode=verify-full"

``dsn`` is a standard PostgreSQL connection string (URL or ``key=value``
form). Use ``sslmode=verify-full`` for any database not on the same host; the
connection carries every policy and registration, including MCP ``env``
values.

.. warning::

   The configuration file now contains a database password. Restrict its
   permissions (for example ``chmod 600``, owned by the gateway's user).

3. Start the gateway
--------------------

On first start against an empty database, the tables are created and the
configuration file's ``[[mcps]]`` and policies are seeded into them.

.. _how-to-use-postgresql-moving-from-sqlite:

Moving from SQLite
------------------

There is no migration tool. Either:

- **re-seed**: make sure ``config.toml`` describes the current registrations
  and policies, and start against the empty PostgreSQL database; or
- **re-create through the API**: export with ``GET /admin/mcps``,
  ``/admin/access-policies`` and ``/admin/filter-policies`` from the old
  gateway, and ``POST`` them to the new one — for MCP servers, the ``name``,
  ``transport``, ``connect`` and ``enabled`` fields of each registration.

Running more than one instance
------------------------------

Each gateway instance caches the state in memory and refreshes it only for
changes made through that instance. Instances sharing one database do not yet
notify each other, so either run one instance, or make all administrative
changes through one and restart the others afterwards.

See also
--------

- :ref:`reference-configuration-3-persistence`
- :doc:`/architecture/decisions/0006-gorm-sqlite-postgres-persistence`
