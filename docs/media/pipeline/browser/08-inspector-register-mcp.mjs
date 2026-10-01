// Pipeline stage: the control plane's OWN MCP surface (ADR-0011) --
// driving /admin/mcp with a real MCP client (Inspector) instead of curl
// or the REST API. list_mcps is a tool on that server; calling it here
// reads the same cache.Registry the REST admin API and config-file
// seeding both populate.
//
// Needs mcplake-admin's bearer token (same ROPC fetch as
// tapes/lib/env.sh's token_for) passed via MCPLAKE_ADMIN_TOKEN.
//
// Inspector 2.9.0 (compose.yaml's pin -- bumped from 2.7.0, which had no
// manual-header field for streamable-http servers at all; see
// docs/media/pipeline/README.md's "Known issues"). The flow: add the
// server, open its Settings, expand "Custom Headers", add
// Authorization: Bearer <token>, then connect. Selectors below are
// verified against a real Inspector 2.9.0 run, not guessed.
import { chromium } from "playwright";

const OUT_DIR = new URL("../out/", import.meta.url);
const pause = (ms) => new Promise((r) => setTimeout(r, ms));

const ADMIN_TOKEN = process.env.MCPLAKE_ADMIN_TOKEN;
if (!ADMIN_TOKEN) {
  throw new Error(
    "Set MCPLAKE_ADMIN_TOKEN first, e.g.: " +
      "export MCPLAKE_ADMIN_TOKEN=$(bash -c 'source ../tapes/lib/env.sh && token_for mcplake-admin')",
  );
}

const browser = await chromium.launch();
const context = await browser.newContext({
  recordVideo: { dir: OUT_DIR.pathname, size: { width: 1280, height: 720 } },
  viewport: { width: 1280, height: 720 },
});
const page = await context.newPage();

// Inspector's own session token, pinned in compose.yaml (see
// docs/getting-started/demo.rst's "MCP Inspector" section) -- distinct
// from the gateway's admin_auth.
await page.goto("http://localhost:6274/?MCP_INSPECTOR_API_TOKEN=mcplake-demo-inspector");
await pause(1200);

await page.getByRole("button", { name: "Add Servers" }).click();
await page.getByText("Add manually", { exact: false }).click();
await pause(500);

await page.getByRole("textbox", { name: "Server ID" }).pressSequentially("mcplake-admin-mcp", { delay: 30 });
await page.getByRole("textbox", { name: "Transport" }).click();
await page.getByRole("option", { name: "streamable-http" }).click();
await pause(300);
await page.getByRole("textbox", { name: "URL" }).pressSequentially("http://localhost:9091/admin/mcp", { delay: 15 });
await pause(500);
await page.getByRole("button", { name: "Add", exact: true }).click();
await pause(1200);

// Open the new card's Settings -- it's the last one in the list.
const settingsLinks = page.getByText("Settings", { exact: true });
await settingsLinks.nth((await settingsLinks.count()) - 1).click();
await pause(800);
await page.getByText("Custom Headers", { exact: true }).click();
await pause(500);
await page.getByRole("button", { name: "+ Add Header" }).click();
await pause(400);
await page.getByPlaceholder("Key").pressSequentially("Authorization", { delay: 30 });
await page.getByPlaceholder("Value").fill(`Bearer ${ADMIN_TOKEN}`); // long/sensitive -- typed instantly, not keystroke-by-keystroke
await pause(500);
await page.keyboard.press("Escape");
await pause(600);

// The connect toggle is a custom Mantine switch; its track label overlaps
// the actual input for pointer-event purposes, so a plain click needs
// force: true.
await page.getByRole("switch").last().click({ force: true });
await pause(2000);

await page.getByText("Tools", { exact: true }).first().click();
await pause(800);
await page.getByText("list_mcps", { exact: true }).click();
await pause(500);
await page.getByRole("button", { name: "Execute Tool" }).click();
await pause(2000);

await page.screenshot({ path: new URL("08-inspector-register-mcp.png", OUT_DIR).pathname });
await pause(500);

await context.close();
await browser.close();

console.log("Recorded. Convert with:");
console.log("  ffmpeg -i out/<generated>.webm -vf 'fps=12,scale=1280:-1' out/08-inspector-register-mcp.gif");
