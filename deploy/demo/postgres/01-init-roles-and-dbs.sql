-- Creates the two databases this demo needs out of one Postgres instance:
--
--   gateway  - the mcplake control-plane's own persistence backend
--              (config.toml's [persistence], driver = "postgres", ADR-0006)
--   demo     - the small dataset the dummy postgres MCP server queries;
--              seeded by 02-demo-seed.sql, which runs after this file.
--
-- Two roles keep the blast radius of each credential obvious: "gateway" only
-- ever touches its own control-plane tables, "demo_reader" only ever reads
-- the demo dataset (and only SELECT, not that "gateway" mounts the MCP
-- subprocess at all).
CREATE ROLE gateway WITH LOGIN PASSWORD 'gateway';
CREATE DATABASE gateway OWNER gateway;

CREATE ROLE demo_reader WITH LOGIN PASSWORD 'demo_reader';
CREATE DATABASE demo OWNER postgres;
