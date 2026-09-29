// Pipeline stage: the control plane's OWN MCP surface (ADR-0011) --
// driving /admin/mcp with a real MCP client (Inspector) instead of curl
// or the REST API. list_mcps and register_mcp are tools on that server;
// calling them here is the same cache.Registry.Register write path the
// REST admin API and config-file seeding both go through.
//
// Needs mcplake-admin's bearer token (fetched via ROPC, same as the
// tapes/lib/env.sh helpers) pasted into Inspector's request headers.
// Selectors are best-effort against MCP Inspector's own UI
// (https://modelcontextprotocol.io/docs/2026-07-28/tools/inspector) --
// expect to adjust these against the version compose.yaml actually pins.
import { chromium } from "playwright";

const OUT_DIR = new URL("../out/", import.meta.url);
const ADMIN_TOKEN = process.env.MCPLAKE_ADMIN_TOKEN;
if (!ADMIN_TOKEN) {
  throw new Error("Set MCPLAKE_ADMIN_TOKEN first, e.g.: " +
    "export MCPLAKE_ADMIN_TOKEN=$(bash -c 'source ../tapes/lib/env.sh && token_for mcplake-admin')");
}

const browser = await chromium.launch();
const context = await browser.newContext({
  recordVideo: { dir: OUT_DIR.pathname, size: { width: 1280, height: 900 } },
  viewport: { width: 1280, height: 900 },
});
const page = await context.newPage();

// Inspector's own session token, pinned in compose.yaml (see docs/DEMO.md's
// "MCP Inspector" section) -- distinct from the gateway's admin_auth.
await page.goto("http://localhost:6274/?MCP_PROXY_AUTH_TOKEN=mcplake-demo-inspector");

await page.getByLabel("Transport Type").selectOption({ label: "Streamable HTTP" });
await page.getByLabel("URL").fill("http://localhost:9091/admin/mcp");
await page.getByRole("button", { name: "Add Header" }).click();
await page.getByLabel("Header Name").last().fill("Authorization");
await page.getByLabel("Header Value").last().fill(`Bearer ${ADMIN_TOKEN}`);
await page.getByRole("button", { name: "Connect" }).click();
await page.getByText("Connected", { exact: false }).waitFor();

await page.getByRole("tab", { name: "Tools" }).click();
await page.getByRole("button", { name: "list_mcps" }).click();
await page.getByRole("button", { name: "Run Tool" }).click();
await page.getByText("employee-directory", { exact: false }).waitFor();

await page.screenshot({ path: new URL("08-inspector-register-mcp.png", OUT_DIR).pathname });

await context.close();
await browser.close();

console.log("Recorded. Convert with:");
console.log("  ffmpeg -i out/<generated>.webm -vf 'fps=12,scale=1280:-1' out/08-inspector-register-mcp.gif");
