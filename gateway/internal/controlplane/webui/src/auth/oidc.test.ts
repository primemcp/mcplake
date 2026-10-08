import { afterEach, describe, expect, it, vi } from "vitest";
import type { AuthConfig, LoginConfig } from "../api/types";
import { authorizeUrl, decodeJwtPayload, exchangeCode, loginConfigOf, readCallback, refreshWith, subjectOf } from "./oidc";

const config: LoginConfig = {
  client_id: "mcplake-admin-ui",
  authorization_endpoint: "https://auth.example.com/authorize",
  token_endpoint: "https://auth.example.com/oauth/token",
  scopes: ["openid", "profile", "email"],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

function jwt(payload: Record<string, unknown>): string {
  const b64 = (o: unknown) => btoa(JSON.stringify(o)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `${b64({ alg: "RS256" })}.${b64(payload)}.signature`;
}

describe("authorizeUrl", () => {
  it("builds an Authorization Code + PKCE request with every required parameter", async () => {
    const url = new URL(
      await authorizeUrl(config, { verifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk", state: "st", redirectUri: "http://gw.local/" }),
    );

    expect(url.origin + url.pathname).toBe("https://auth.example.com/authorize");
    expect(url.searchParams.get("response_type")).toBe("code");
    expect(url.searchParams.get("client_id")).toBe("mcplake-admin-ui");
    expect(url.searchParams.get("redirect_uri")).toBe("http://gw.local/");
    expect(url.searchParams.get("scope")).toBe("openid profile email");
    expect(url.searchParams.get("state")).toBe("st");
    expect(url.searchParams.get("code_challenge_method")).toBe("S256");
    // The challenge, never the verifier -- sending the verifier here would
    // defeat the whole point of PKCE.
    expect(url.searchParams.get("code_challenge")).toBe("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM");
    expect(url.search).not.toContain("dBjftJeZ4CVP");
  });

  it("keeps query parameters the provider's own endpoint already carries", async () => {
    const withQuery = { ...config, authorization_endpoint: "https://auth.example.com/authorize?tenant=acme" };

    const url = new URL(await authorizeUrl(withQuery, { verifier: "v", state: "s", redirectUri: "http://gw.local/" }));

    expect(url.searchParams.get("tenant")).toBe("acme");
    expect(url.searchParams.get("response_type")).toBe("code");
  });
});

describe("readCallback", () => {
  it("reads a successful redirect", () => {
    expect(readCallback("http://gw.local/?code=abc&state=st")).toEqual({ kind: "code", code: "abc", state: "st" });
  });

  it("reads a provider error, preferring its description", () => {
    const got = readCallback("http://gw.local/?error=access_denied&error_description=User%20said%20no&state=st");

    expect(got).toEqual({ kind: "error", message: "User said no", state: "st" });
  });

  it("falls back to the bare error code when there is no description", () => {
    // state is null here, which is itself the signal the caller checks:
    // an error response is required to echo it (RFC 6749 4.1.2.1).
    expect(readCallback("http://gw.local/?error=access_denied")).toEqual({
      kind: "error",
      message: "access_denied",
      state: null,
    });
  });

  it("reports an ordinary page load as no callback at all", () => {
    expect(readCallback("http://gw.local/")).toEqual({ kind: "none" });
    expect(readCallback("http://gw.local/?other=1")).toEqual({ kind: "none" });
  });

  it("treats a code with no state as a callback -- so the caller rejects it rather than ignoring it", () => {
    expect(readCallback("http://gw.local/?code=abc")).toEqual({ kind: "code", code: "abc", state: null });
  });
});

describe("exchangeCode", () => {
  it("posts the code and verifier as form-encoded credentials-free parameters", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ access_token: "at", refresh_token: "rt", expires_in: 300, token_type: "Bearer" }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const tokens = await exchangeCode(config, { code: "abc", verifier: "ver", redirectUri: "http://gw.local/" });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://auth.example.com/oauth/token");
    expect(init.method).toBe("POST");
    expect(init.headers["Content-Type"]).toBe("application/x-www-form-urlencoded");
    const sent = new URLSearchParams(init.body as string);
    expect(sent.get("grant_type")).toBe("authorization_code");
    expect(sent.get("code")).toBe("abc");
    expect(sent.get("code_verifier")).toBe("ver");
    expect(sent.get("redirect_uri")).toBe("http://gw.local/");
    expect(sent.get("client_id")).toBe("mcplake-admin-ui");
    expect(sent.get("client_secret")).toBeNull();

    expect(tokens.access_token).toBe("at");
    expect(tokens.refresh_token).toBe("rt");
    expect(tokens.expires_in).toBe(300);
  });

  it("surfaces the provider's own error text, which is what tells an operator what to fix", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: "invalid_grant", error_description: "code expired" }), {
          status: 400,
          headers: { "content-type": "application/json" },
        }),
      ),
    );

    await expect(exchangeCode(config, { code: "abc", verifier: "v", redirectUri: "http://gw.local/" })).rejects.toThrow(
      /code expired/,
    );
  });

  it("fails loudly when the provider answers 200 without an access token", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ id_token: "only-an-id-token" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );

    await expect(exchangeCode(config, { code: "abc", verifier: "v", redirectUri: "http://gw.local/" })).rejects.toThrow(
      /access_token/,
    );
  });
});

describe("refreshWith", () => {
  it("posts the refresh grant", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ access_token: "at2", expires_in: 300 }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const tokens = await refreshWith(config, "rt");

    const sent = new URLSearchParams(fetchMock.mock.calls[0][1].body as string);
    expect(sent.get("grant_type")).toBe("refresh_token");
    expect(sent.get("refresh_token")).toBe("rt");
    expect(sent.get("client_id")).toBe("mcplake-admin-ui");
    expect(tokens.access_token).toBe("at2");
  });
});

describe("decodeJwtPayload / subjectOf", () => {
  it("decodes a payload without verifying anything", () => {
    expect(decodeJwtPayload(jwt({ sub: "u-1", role: "admin" }))).toEqual({ sub: "u-1", role: "admin" });
  });

  it("returns null rather than throwing for anything that isn't a readable JWT", () => {
    expect(decodeJwtPayload("not.a.jwt")).toBeNull();
    expect(decodeJwtPayload("opaque-token")).toBeNull();
    expect(decodeJwtPayload("")).toBeNull();
  });

  it("prefers a human-readable identity claim, falling back to sub", () => {
    expect(subjectOf(jwt({ sub: "u-1", email: "ops@acme.io" }))).toBe("ops@acme.io");
    expect(subjectOf(jwt({ sub: "u-1", preferred_username: "ops" }))).toBe("ops");
    expect(subjectOf(jwt({ sub: "u-1" }))).toBe("u-1");
    expect(subjectOf("opaque-token")).toBeNull();
  });
});

describe("loginConfigOf", () => {
  it("narrows a complete answer, defaulting scopes the gateway didn't send", () => {
    const full: AuthConfig = { auth_required: true, client_id: "c", authorization_endpoint: "https://a/", token_endpoint: "https://t/" };

    expect(loginConfigOf(full)).toEqual({
      client_id: "c",
      authorization_endpoint: "https://a/",
      token_endpoint: "https://t/",
      scopes: ["openid", "profile", "email"],
    });
  });

  it("returns null when any coordinate is missing -- there is no half-usable login", () => {
    expect(loginConfigOf({ auth_required: true })).toBeNull();
    expect(loginConfigOf({ auth_required: true, client_id: "c", authorization_endpoint: "https://a/" })).toBeNull();
  });
});
