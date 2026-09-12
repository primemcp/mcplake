import { afterEach, describe, expect, it } from "vitest";
import { clearSession, isExpired, loadSession, saveSession, sessionFrom, takePending, savePending } from "./session";

afterEach(() => {
  sessionStorage.clear();
  localStorage.clear();
});

describe("sessionFrom", () => {
  it("turns a token response into a session, resolving expires_in against now", () => {
    const session = sessionFrom({ access_token: "at", refresh_token: "rt", expires_in: 300 }, 1_000_000);

    expect(session).toEqual({ accessToken: "at", refreshToken: "rt", expiresAt: 1_000_000 + 300_000 });
  });

  it("leaves expiry unknown when the provider didn't say", () => {
    expect(sessionFrom({ access_token: "at" }, 1_000_000).expiresAt).toBeNull();
  });

  it("keeps the previous refresh token when a refresh response omits one", () => {
    const session = sessionFrom({ access_token: "at2", expires_in: 60 }, 0, "rt-from-before");

    expect(session.refreshToken).toBe("rt-from-before");
  });
});

describe("isExpired", () => {
  const session = { accessToken: "at", refreshToken: null, expiresAt: 10_000 };

  it("is false well before expiry and true after it", () => {
    expect(isExpired(session, 0)).toBe(false);
    expect(isExpired(session, 20_000)).toBe(true);
  });

  // Renewing a few seconds early avoids losing a request to a token that
  // expires between our check and the gateway's.
  it("treats a token expiring within the skew window as already expired", () => {
    expect(isExpired(session, 9_000)).toBe(true);
  });

  it("treats an unknown expiry as not expired -- the gateway's 401 decides instead", () => {
    expect(isExpired({ accessToken: "at", refreshToken: null, expiresAt: null }, 10 ** 12)).toBe(false);
  });
});

describe("session storage", () => {
  const session = { accessToken: "at", refreshToken: "rt", expiresAt: 10_000 };

  it("round-trips through sessionStorage", () => {
    saveSession(session);

    expect(loadSession()).toEqual(session);
  });

  // sessionStorage, not localStorage: the token dies with the tab, and is
  // never shared with another tab or left behind after closing (ADR-0014).
  it("stores the token in sessionStorage only", () => {
    saveSession(session);

    expect(JSON.stringify(Object.values(localStorage))).not.toContain("at");
    expect(JSON.stringify(Object.values(sessionStorage))).toContain("at");
  });

  it("clears", () => {
    saveSession(session);
    clearSession();

    expect(loadSession()).toBeNull();
  });

  it("ignores stored junk instead of crashing the app on boot", () => {
    sessionStorage.setItem("mcplake:auth:session", "{not json");
    expect(loadSession()).toBeNull();

    sessionStorage.setItem("mcplake:auth:session", JSON.stringify({ accessToken: 42 }));
    expect(loadSession()).toBeNull();
  });
});

describe("pending PKCE state", () => {
  it("round-trips and is consumed exactly once -- a replayed callback finds nothing", () => {
    savePending({ verifier: "v", state: "s" });

    expect(takePending()).toEqual({ verifier: "v", state: "s" });
    expect(takePending()).toBeNull();
  });

  it("keeps the verifier out of localStorage too", () => {
    savePending({ verifier: "v", state: "s" });

    expect(JSON.stringify(Object.values(localStorage))).not.toContain("\"v\"");
  });
});
