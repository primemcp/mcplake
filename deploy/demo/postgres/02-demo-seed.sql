-- Seed data for the "demo" database the dummy postgres MCP server (see
-- deploy/demo/config.toml's [[mcps]] entry) queries. \connect switches this
-- script onto "demo" — 01-init-roles-and-dbs.sql already created it and runs
-- first (docker-entrypoint-initdb.d applies *.sql in filename order).
\connect demo

CREATE TABLE widgets (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    sku         TEXT NOT NULL UNIQUE,
    price_cents INTEGER NOT NULL
);

INSERT INTO widgets (name, sku, price_cents) VALUES
    ('Left-Handed Smoke Shifter', 'LHSS-001', 1999),
    ('Bucket of Steam',           'BOS-002',   499),
    ('Sky Hook',                  'SKH-003',  2999),
    ('Tartan Paint',              'TP-004',   1299);

-- demo_reader is the credential the postgres MCP server subprocess connects
-- with; the server also wraps every query in a read-only transaction itself,
-- but granting only SELECT here means that's defense in depth, not the only
-- thing standing between a caller and a write.
GRANT USAGE ON SCHEMA public TO demo_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO demo_reader;
