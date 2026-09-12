import { describe, expect, it } from "vitest";
import { base64UrlEncode, codeChallengeOf, newState, newVerifier } from "./pkce";

describe("newVerifier", () => {
  it("stays inside RFC 7636's 43..128 unreserved-character range", () => {
    const verifier = newVerifier();

    expect(verifier.length).toBeGreaterThanOrEqual(43);
    expect(verifier.length).toBeLessThanOrEqual(128);
    expect(verifier).toMatch(/^[A-Za-z0-9\-._~]+$/);
  });

  it("never repeats -- it is the only thing that makes the code redeemable by this tab alone", () => {
    const seen = new Set(Array.from({ length: 50 }, () => newVerifier()));

    expect(seen.size).toBe(50);
  });
});

describe("newState", () => {
  it("is unguessable and URL-safe -- it is the CSRF defence on the callback", () => {
    const seen = new Set(Array.from({ length: 50 }, () => newState()));

    expect(seen.size).toBe(50);
    expect(newState()).toMatch(/^[A-Za-z0-9\-._~]{32,}$/);
  });
});

describe("base64UrlEncode", () => {
  it("encodes without padding and without + or /", () => {
    // 0xfb 0xff 0xfe is base64 "+//+" -- the exact bytes that expose a
    // standard-base64 encoder pretending to be base64url.
    const encoded = base64UrlEncode(new Uint8Array([0xfb, 0xff, 0xfe]));

    expect(encoded).toBe("-__-");
    expect(encoded).not.toContain("=");
  });
});

describe("codeChallengeOf", () => {
  // The one worked example in RFC 7636 (Appendix B).
  it("matches RFC 7636's own S256 test vector", async () => {
    const challenge = await codeChallengeOf("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk");

    expect(challenge).toBe("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM");
  });

  it("is deterministic for a given verifier and differs between verifiers", async () => {
    const [a, b, again] = await Promise.all([
      codeChallengeOf("verifier-a"),
      codeChallengeOf("verifier-b"),
      codeChallengeOf("verifier-a"),
    ]);

    expect(a).toBe(again);
    expect(a).not.toBe(b);
  });
});
