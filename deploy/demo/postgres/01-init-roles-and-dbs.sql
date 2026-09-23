-- The mcplake control-plane's own persistence backend (config.toml's
-- [persistence], driver = "postgres", ADR-0006).
--
-- This database is the gateway's alone. The `demo` database that
-- 02-demo-data.sql creates is what the postgres-mcp container serves, and
-- the two are kept apart on purpose: a downstream MCP that could read the
-- gateway's own access_policies and filter_policies tables would be reading
-- the rules that govern it. The employee-directory MCP
-- (deploy/demo/mcp-servers/) touches neither -- its data is in-memory.
CREATE ROLE gateway WITH LOGIN PASSWORD 'gateway';
CREATE DATABASE gateway OWNER gateway;
