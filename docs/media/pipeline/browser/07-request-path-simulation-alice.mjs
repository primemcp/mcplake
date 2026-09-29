// Pipeline stage: the whole request pipeline, visualized -- Users & access
// -> demo-reader -> "Request path" tab (RequestPathTab.tsx). This tab is
// the one place the UI already renders the gateway's own pipeline as
// nodes: Client/Agent -> Auth Validator -> Access Check -> Response
// Filter, each with its own live status. Recording this IS recording the
// pipeline, in the project's own words, not a paraphrase of it.
//
// "demo-reader" ($.role ~ ^db-reader$) is the AccessPolicy alice's token
// actually matches -- this screen lists policies, not literal usernames
// (docs/features/admin-webui.md's "Users & access"). Selecting it
// pre-fills a decoded-JWT payload with role=db-reader and defaults the
// simulated call to employee-directory/get_employee, and the simulation
// runs reactively -- no separate "run" click needed.
//
// Run against the live compose demo. Node labels ("Auth Validator",
// "Access Check", "Response Filter") are taken directly from
// RequestPathTab.test.tsx -- real, not guessed.
import { chromium } from "playwright";

const OUT_DIR = new URL("../out/", import.meta.url);
const pause = (ms) => new Promise((r) => setTimeout(r, ms));

const browser = await chromium.launch();
const context = await browser.newContext({
  recordVideo: { dir: OUT_DIR.pathname, size: { width: 1280, height: 900 } },
  viewport: { width: 1280, height: 900 },
});
const page = await context.newPage();

await page.goto("http://localhost:9091/");
await pause(800);
await page.getByRole("button", { name: "Sign in" }).click();
await page.waitForURL(/realms\/mcplake\/protocol\/openid-connect\/auth/);
await pause(500);
await page.getByLabel("Username or email").pressSequentially("mcplake-admin", { delay: 50 });
await page.getByRole("textbox", { name: "Password" }).pressSequentially("mcplake-admin", { delay: 50 });
await pause(400);
await page.getByRole("button", { name: "Sign In" }).click();
await page.waitForURL("http://localhost:9091/**");
await page.getByText("MCP connections", { exact: false }).first().waitFor();
await pause(1000);

await page.getByRole("button", { name: "Users & access" }).click();
await pause(800);
await page.getByText("demo-reader", { exact: false }).first().click();
await pause(800);
await page.getByRole("tab", { name: "Request path" }).click();
await pause(1500);

// The simulation already ran reactively (default payload matches this
// policy, default endpoint/tool is employee-directory/get_employee).
// Click the Response Filter node for the "N fields dropped" detail.
await page.getByText("Response Filter", { exact: false }).first().click();
await pause(2000);

await page.screenshot({ path: new URL("07-request-path-simulation-alice.png", OUT_DIR).pathname });
await pause(500);

await context.close();
await browser.close();

console.log("Recorded. Convert with:");
console.log("  ffmpeg -i out/<generated>.webm -vf 'fps=12,scale=1280:-1' out/07-request-path-simulation-alice.gif");
