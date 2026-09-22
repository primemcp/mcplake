import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { AuthConfig } from "../api/types";
import { AuthProvider } from "./AuthProvider";
import { SignedInAs } from "./SignedInAs";
import * as browser from "./browser";
import { loadSession, saveSession } from "./session";

vi.mock("./browser", async (importOriginal) => ({
  ...(await importOriginal<typeof browser>()),
  currentHref: vi.fn(() => "http://gw.local/"),
  redirectUri: vi.fn(() => "http://gw.local/"),
  redirectTo: vi.fn(),
  stripQuery: vi.fn(),
}));

const LOGIN: AuthConfig = {
  auth_required: true,
  issuer: "https://auth.example.com",
  client_id: "mcplake-admin-ui",
  authorization_endpoint: "https://auth.example.com/authorize",
  token_endpoint: "https://auth.example.com/oauth/token",
  scopes: ["openid", "profile", "email"],
};

function jwt(payload: Record<string, unknown>): string {
  const b64 = (o: unknown) => btoa(JSON.stringify(o)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `${b64({ alg: "RS256" })}.${b64(payload)}.sig`;
}

const ADMIN_TOKEN = jwt({ sub: "u-1", email: "ops@acme.io" });

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
}

type Stub = {
  fetch: ReturnType<typeof vi.fn>;
  /** What GET /admin/mcps answers -- how the tests provoke a real 401/403
   * through the same path the app's own screens use. */
  mcpsStatus: number;
  tokenResponse: () => Response;
};

/** Routes the three calls in play: the gateway's bootstrap endpoint, the
 * provider's token endpoint, and one ordinary admin request. */
function stubFetch(authConfig: AuthConfig): Stub {
  const stub: Stub = { fetch: vi.fn(), mcpsStatus: 200, tokenResponse: () => json({}) };
  stub.fetch = vi.fn((url: string) => {
    if (url === "/admin/auth/config") return Promise.resolve(json(authConfig));
    if (url === LOGIN.token_endpoint) return Promise.resolve(stub.tokenResponse());
    if (url === "/admin/mcps") {
      return Promise.resolve(stub.mcpsStatus === 200 ? json([]) : json({ error: "nope", message: "nope" }, stub.mcpsStatus));
    }
    throw new Error(`unexpected fetch: ${url}`);
  });
  vi.stubGlobal("fetch", stub.fetch);
  return stub;
}

// Mirrors how the real app composes them: the provider gates the tree,
// and the sidebar inside it renders the signed-in identity.
const app = (
  <>
    <div>MCP connections</div>
    <SignedInAs />
  </>
);

/** Makes one real admin request and returns the bearer token it carried,
 * which is what the gateway would actually have seen. */
async function tokenSentOnNextRequest(stub: Stub): Promise<string | null> {
  stub.fetch.mockClear();
  await api.listMCPs().catch(() => undefined);
  const call = stub.fetch.mock.calls.find(([url]) => url === "/admin/mcps");
  const header = (call?.[1]?.headers as Record<string, string> | undefined)?.Authorization;
  return header ? header.replace("Bearer ", "") : null;
}

/** Provokes the given status from a real admin call, then lets React
 * flush whatever the auth layer decided to do about it. */
async function adminRequestFails(stub: Stub, status: number) {
  stub.mcpsStatus = status;
  await api.listMCPs().catch(() => undefined);
  await waitFor(() => expect(true).toBe(true));
}

function tokenEndpointCalls(stub: Stub) {
  return stub.fetch.mock.calls.filter(([url]) => url === LOGIN.token_endpoint);
}

beforeEach(() => {
  vi.mocked(browser.currentHref).mockReturnValue("http://gw.local/");
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  sessionStorage.clear();
});

describe("AuthProvider", () => {
  it("renders the app untouched when the gateway says no auth is required", async () => {
    stubFetch({ auth_required: false });

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText("MCP connections")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Sign in/ })).not.toBeInTheDocument();
  });

  it("sends no Authorization header when auth is not required", async () => {
    const stub = stubFetch({ auth_required: false });
    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText("MCP connections");

    expect(await tokenSentOnNextRequest(stub)).toBeNull();
  });

  it("shows the login screen, naming the provider, when auth is required and nobody is signed in", async () => {
    stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByRole("button", { name: /Sign in/ })).toBeInTheDocument();
    expect(screen.getByText(/auth\.example\.com/)).toBeInTheDocument();
    expect(screen.queryByText("MCP connections")).not.toBeInTheDocument();
  });

  it("signing in redirects to the provider with a PKCE challenge and keeps the verifier here", async () => {
    const user = userEvent.setup();
    stubFetch(LOGIN);
    render(<AuthProvider>{app}</AuthProvider>);

    await user.click(await screen.findByRole("button", { name: /Sign in/ }));

    await waitFor(() => expect(browser.redirectTo).toHaveBeenCalled());
    const target = new URL(vi.mocked(browser.redirectTo).mock.calls[0][0]);
    expect(target.origin + target.pathname).toBe(LOGIN.authorization_endpoint);
    expect(target.searchParams.get("code_challenge_method")).toBe("S256");
    expect(target.searchParams.get("redirect_uri")).toBe("http://gw.local/");

    const pending = JSON.parse(sessionStorage.getItem("mcplake:auth:pending") as string);
    expect(pending.state).toBe(target.searchParams.get("state"));
    expect(target.search).not.toContain(pending.verifier);
  });

  it("completes the callback: exchanges the code, shows the app, and scrubs the URL", async () => {
    sessionStorage.setItem("mcplake:auth:pending", JSON.stringify({ verifier: "ver", state: "st" }));
    vi.mocked(browser.currentHref).mockReturnValue("http://gw.local/?code=abc&state=st");
    const stub = stubFetch(LOGIN);
    stub.tokenResponse = () => json({ access_token: ADMIN_TOKEN, expires_in: 300 });

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText("MCP connections")).toBeInTheDocument();
    const sent = new URLSearchParams(tokenEndpointCalls(stub)[0][1].body as string);
    expect(sent.get("code")).toBe("abc");
    expect(sent.get("code_verifier")).toBe("ver");
    expect(browser.stripQuery).toHaveBeenCalled();
    expect(loadSession()?.accessToken).toBe(ADMIN_TOKEN);
    expect(await tokenSentOnNextRequest(stub)).toBe(ADMIN_TOKEN);
  });

  // Without this check, anyone could hand an operator a link carrying
  // their own authorization code and have the UI adopt that session.
  it("refuses a callback whose state doesn't match the one this tab stored", async () => {
    sessionStorage.setItem("mcplake:auth:pending", JSON.stringify({ verifier: "ver", state: "st" }));
    vi.mocked(browser.currentHref).mockReturnValue("http://gw.local/?code=abc&state=ATTACKER");
    const stub = stubFetch(LOGIN);
    stub.tokenResponse = () => json({ access_token: ADMIN_TOKEN });

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/could not be verified/i)).toBeInTheDocument();
    expect(tokenEndpointCalls(stub)).toHaveLength(0);
    expect(screen.queryByText("MCP connections")).not.toBeInTheDocument();
  });

  it("refuses a callback when this tab has no pending sign-in at all", async () => {
    vi.mocked(browser.currentHref).mockReturnValue("http://gw.local/?code=abc&state=st");
    const stub = stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/could not be verified/i)).toBeInTheDocument();
    expect(tokenEndpointCalls(stub)).toHaveLength(0);
  });

  it("surfaces a provider-side refusal instead of a blank login screen", async () => {
    // A refusal from a sign-in this tab actually started: the state matches,
    // so the provider's own words are trusted and shown.
    sessionStorage.setItem("mcplake:auth:pending", JSON.stringify({ verifier: "ver", state: "st" }));
    vi.mocked(browser.currentHref).mockReturnValue(
      "http://gw.local/?error=access_denied&error_description=User+said+no&state=st",
    );
    stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/User said no/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Sign in/ })).toBeInTheDocument();
  });

  it("surfaces a failed token exchange", async () => {
    sessionStorage.setItem("mcplake:auth:pending", JSON.stringify({ verifier: "ver", state: "st" }));
    vi.mocked(browser.currentHref).mockReturnValue("http://gw.local/?code=abc&state=st");
    const stub = stubFetch(LOGIN);
    stub.tokenResponse = () => json({ error: "invalid_grant", error_description: "code expired" }, 400);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/code expired/)).toBeInTheDocument();
  });

  it("restores a stored session on reload without going back to the provider", async () => {
    saveSession({ accessToken: ADMIN_TOKEN, refreshToken: null, expiresAt: Date.now() + 300_000 });
    stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText("MCP connections")).toBeInTheDocument();
    expect(browser.redirectTo).not.toHaveBeenCalled();
  });

  it("shows who is signed in, and signing out clears the session and returns to the login screen", async () => {
    const user = userEvent.setup();
    saveSession({ accessToken: ADMIN_TOKEN, refreshToken: null, expiresAt: Date.now() + 300_000 });
    stubFetch(LOGIN);
    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText("MCP connections");

    expect(screen.getByText("ops@acme.io")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Sign out/ }));

    expect(await screen.findByRole("button", { name: /Sign in/ })).toBeInTheDocument();
    expect(loadSession()).toBeNull();
  });

  it("a 401 from the admin API ends the session and asks for a new sign-in", async () => {
    saveSession({ accessToken: ADMIN_TOKEN, refreshToken: null, expiresAt: Date.now() + 300_000 });
    const stub = stubFetch(LOGIN);
    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText("MCP connections");

    await adminRequestFails(stub, 401);

    expect(await screen.findByRole("button", { name: /Sign in/ })).toBeInTheDocument();
    expect(loadSession()).toBeNull();
  });

  // A 403 means admin_auth.match rejected these claims. Signing in again
  // as the same person cannot help, so offering "Sign in" would be a loop.
  it("a 403 shows the not-an-admin screen naming the subject, with no sign-in button", async () => {
    saveSession({ accessToken: ADMIN_TOKEN, refreshToken: null, expiresAt: Date.now() + 300_000 });
    const stub = stubFetch(LOGIN);
    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText("MCP connections");

    await adminRequestFails(stub, 403);

    expect(await screen.findByText(/not an admin/i)).toBeInTheDocument();
    expect(screen.getByText("ops@acme.io")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Sign in/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Sign out/ })).toBeInTheDocument();
  });

  it("refreshes an expired token before the request rather than bouncing the operator", async () => {
    saveSession({ accessToken: "stale", refreshToken: "rt", expiresAt: Date.now() - 1 });
    const stub = stubFetch(LOGIN);
    stub.tokenResponse = () => json({ access_token: ADMIN_TOKEN, expires_in: 300 });
    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText("MCP connections");

    expect(await tokenSentOnNextRequest(stub)).toBe(ADMIN_TOKEN);
    expect(new URLSearchParams(tokenEndpointCalls(stub)[0][1].body as string).get("grant_type")).toBe("refresh_token");
    expect(loadSession()?.accessToken).toBe(ADMIN_TOKEN);
  });

  it("refreshes once for concurrent requests, not once per request", async () => {
    saveSession({ accessToken: "stale", refreshToken: "rt", expiresAt: Date.now() - 1 });
    const stub = stubFetch(LOGIN);
    stub.tokenResponse = () => json({ access_token: ADMIN_TOKEN, expires_in: 300 });
    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText("MCP connections");
    stub.fetch.mockClear();

    await Promise.all([api.listMCPs(), api.listMCPs(), api.listMCPs()]);

    expect(tokenEndpointCalls(stub)).toHaveLength(1);
  });

  it("an expired token that cannot be refreshed returns the operator to the login screen", async () => {
    saveSession({ accessToken: "stale", refreshToken: "rt", expiresAt: Date.now() - 1 });
    const stub = stubFetch(LOGIN);
    stub.tokenResponse = () => json({ error: "invalid_grant" }, 400);
    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText("MCP connections");

    expect(await tokenSentOnNextRequest(stub)).toBeNull();

    expect(await screen.findByRole("button", { name: /Sign in/ })).toBeInTheDocument();
    expect(loadSession()).toBeNull();
  });

  // Admin auth on, [admin_auth.login] unset: there is nothing to redirect
  // to, so say what the operator has to fix rather than offer a dead button.
  it("explains the misconfiguration when auth is required but no login flow is configured", async () => {
    stubFetch({ auth_required: true, issuer: "https://auth.example.com" });

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/admin_auth\.login/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Sign in/ })).not.toBeInTheDocument();
    expect(screen.queryByText("MCP connections")).not.toBeInTheDocument();
  });

  it("offers a retry when the gateway itself can't be reached", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network down")));

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByRole("button", { name: /Retry/ })).toBeInTheDocument();
  });
});

// RFC 6749 §4.1.2.1 requires `state` on the error response too, and the
// client to verify it. Without that, anyone can hand an operator a link
// that renders their own text inside the gateway's own sign-in card --
// escaped by React, so not XSS, but a convincing phishing surface on a
// trusted origin, with the query string already scrubbed from the address
// bar by the time it is read.
describe("AuthProvider: error callbacks are verified like successful ones", () => {
  const attackerText = "Admin SSO moved, sign in at https://gw-sso.attacker.tld";

  it("refuses a provider error whose state doesn't match this tab's", async () => {
    sessionStorage.setItem("mcplake:auth:pending", JSON.stringify({ verifier: "ver", state: "st" }));
    vi.mocked(browser.currentHref).mockReturnValue(
      `http://gw.local/?error=invalid_request&error_description=${encodeURIComponent(attackerText)}&state=ATTACKER`,
    );
    stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/could not be verified/i)).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(attackerText))).not.toBeInTheDocument();
  });

  it("refuses a provider error when this tab has no sign-in pending at all", async () => {
    vi.mocked(browser.currentHref).mockReturnValue(
      `http://gw.local/?error=access_denied&error_description=${encodeURIComponent(attackerText)}`,
    );
    stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/could not be verified/i)).toBeInTheDocument();
    expect(screen.queryByText(new RegExp(attackerText))).not.toBeInTheDocument();
  });

  // An unsolicited callback must not consume the pending PKCE state, or it
  // breaks a real sign-in that is in flight in the same tab.
  it("leaves a pending sign-in intact when it refuses an unsolicited callback", async () => {
    sessionStorage.setItem("mcplake:auth:pending", JSON.stringify({ verifier: "ver", state: "st" }));
    vi.mocked(browser.currentHref).mockReturnValue("http://gw.local/?error=access_denied&state=ATTACKER");
    stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);
    await screen.findByText(/could not be verified/i);

    expect(JSON.parse(sessionStorage.getItem("mcplake:auth:pending") as string)).toEqual({
      verifier: "ver",
      state: "st",
    });
  });

  // A genuine refusal by the provider still shows the provider's own words,
  // which is what tells an operator whether to fix a client registration or
  // just try again.
  it("shows the provider's own message for a verified error callback", async () => {
    sessionStorage.setItem("mcplake:auth:pending", JSON.stringify({ verifier: "ver", state: "st" }));
    vi.mocked(browser.currentHref).mockReturnValue(
      "http://gw.local/?error=access_denied&error_description=User+said+no&state=st",
    );
    stubFetch(LOGIN);

    render(<AuthProvider>{app}</AuthProvider>);

    expect(await screen.findByText(/User said no/)).toBeInTheDocument();
    expect(sessionStorage.getItem("mcplake:auth:pending")).toBeNull();
  });
});
