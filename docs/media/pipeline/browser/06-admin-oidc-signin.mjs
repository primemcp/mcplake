// Pipeline stage: AUTH, from the admin UI's side (ADR-0014).
//
// Opens the embedded admin web UI cold (no session yet), lets it redirect
// to Keycloak, signs in as mcplake-admin, and follows the redirect back --
// the one flow in the demo that's a real Authorization Code + PKCE round
// trip through a browser, not a ROPC curl. Records video (converted to
// .gif afterwards -- see ../README.md) and takes a still of the
// authenticated MCP connections screen as the payoff frame.
//
// Run against the live compose demo (`docker compose up`, http://localhost:9091).
// On a real, fast localhost stack this whole flow completes in under two
// seconds -- too fast to read as a gif -- so deliberate pauses and typed
// (not instantly filled) credentials pace it the same way the tapes/
// scenarios do with `Sleep`.
import { chromium } from "playwright";

const OUT_DIR = new URL("../out/", import.meta.url);
const pause = (ms) => new Promise((r) => setTimeout(r, ms));

const browser = await chromium.launch();
const context = await browser.newContext({
  recordVideo: { dir: OUT_DIR.pathname, size: { width: 1280, height: 800 } },
  viewport: { width: 1280, height: 800 },
});
const page = await context.newPage();

await page.goto("http://localhost:9091/");
await pause(1000);

// LoginScreen (AuthScreens.tsx) requires an explicit click before it
// redirects -- it never auto-navigates the operator away on load.
await page.getByRole("button", { name: "Sign in" }).click();
await page.waitForURL(/realms\/mcplake\/protocol\/openid-connect\/auth/);
await pause(800);

await page.getByLabel("Username or email").pressSequentially("mcplake-admin", { delay: 60 });
await pause(400);
await page.getByRole("textbox", { name: "Password" }).pressSequentially("mcplake-admin", { delay: 60 });
await pause(500);
await page.getByRole("button", { name: "Sign In" }).click();

// Back on the gateway's own origin, authenticated.
await page.waitForURL("http://localhost:9091/**");
await page.getByText("MCP connections", { exact: false }).first().waitFor();
await pause(1500);
await page.screenshot({ path: new URL("06-admin-oidc-signin.png", OUT_DIR).pathname });
await pause(500);

await context.close();
await browser.close();

console.log("Recorded. Convert the .webm this produced with:");
console.log("  ffmpeg -i out/<generated>.webm -vf 'fps=12,scale=1280:-1' out/06-admin-oidc-signin.gif");
