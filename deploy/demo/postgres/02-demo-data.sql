-- The database the `postgres-mcp` container serves (compose.yaml's
-- DATABASE_URI). Separate from the `gateway` database created in
-- 01-init-roles-and-dbs.sql, which is the control plane's own
-- [persistence] and must not be reachable by a downstream MCP: an MCP that
-- could read the gateway's own access_policies table would be reading the
-- rules that govern it.
--
-- demo_reader is granted SELECT and nothing else. postgres-mcp also runs
-- with --access-mode=restricted, so this is the second of two independent
-- limits; the gateway's access_policies are the third and the only one an
-- operator manages at runtime.
--
-- Note: docker-entrypoint-initdb.d only runs on an EMPTY data directory.
-- On a volume that survived an earlier `up`, none of this happens -- see
-- docs/DEMO.md, "Upgrading or resetting the demo".
CREATE ROLE demo_reader WITH LOGIN PASSWORD 'demo_reader';
CREATE DATABASE demo OWNER postgres;

\connect demo

CREATE TABLE customers (
    id             serial PRIMARY KEY,
    company        text    NOT NULL,
    contact_email  text    NOT NULL,
    region         text    NOT NULL,
    plan           text    NOT NULL,
    seats          integer NOT NULL,
    arr_usd        integer NOT NULL
);

CREATE TABLE support_tickets (
    id          serial PRIMARY KEY,
    customer_id integer NOT NULL REFERENCES customers (id),
    opened_on   date    NOT NULL,
    severity    text    NOT NULL,
    status      text    NOT NULL,
    subject     text    NOT NULL
);

-- Fictional, and deliberately a different domain from the employee
-- directory the stdio MCP serves: two MCPs that obviously are not the same
-- server make the per-MCP access grants in config.toml read as something
-- other than an accident.
INSERT INTO customers (company, contact_email, region, plan, seats, arr_usd) VALUES
    ('Northwind Trading',  'ops@northwind.example',    'EMEA',   'enterprise', 420, 318000),
    ('Blue Harbor Labs',   'it@blueharbor.example',    'AMER',   'business',    85,  61200),
    ('Kestrel Analytics',  'admin@kestrel.example',    'AMER',   'enterprise', 310, 245000),
    ('Sable & Finch',      'tech@sablefinch.example',  'EMEA',   'starter',     12,   7200),
    ('Meridian Freight',   'ops@meridian.example',     'APAC',   'business',   140, 102000);

INSERT INTO support_tickets (customer_id, opened_on, severity, status, subject) VALUES
    (1, '2026-08-14', 'high',   'open',     'Bulk export times out past 50k rows'),
    (1, '2026-09-02', 'low',    'resolved', 'SSO metadata refresh'),
    (2, '2026-09-11', 'medium', 'open',     'Webhook retries arrive out of order'),
    (3, '2026-07-29', 'high',   'resolved', 'Read replica lag during nightly sync'),
    (3, '2026-09-18', 'medium', 'open',     'Custom report scheduling in UTC only'),
    (5, '2026-09-20', 'low',    'open',     'Add APAC data residency option');

GRANT CONNECT ON DATABASE demo TO demo_reader;
GRANT USAGE ON SCHEMA public TO demo_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO demo_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO demo_reader;
