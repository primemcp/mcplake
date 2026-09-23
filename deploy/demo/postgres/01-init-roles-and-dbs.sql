-- The mcplake control-plane's own persistence backend (config.toml's
-- [persistence], driver = "postgres", ADR-0006). Nothing else in this demo
-- uses Postgres: the dummy MCP server (deploy/demo/mcp-servers/) keeps its
-- own in-memory seed data, deliberately not a passthrough over a real
-- datastore -- see docs/DEMO.md.
CREATE ROLE gateway WITH LOGIN PASSWORD 'gateway';
CREATE DATABASE gateway OWNER gateway;
