/**
 * PKCE (RFC 7636) primitives for the admin UI's OIDC login, built on
 * WebCrypto so the flow needs no dependency of its own. See ADR-0014.
 *
 * The gateway's UI is a public client: its bundle is readable by anyone
 * who loads the page, so it holds no client secret. PKCE is what makes
 * that safe — the authorization code is only redeemable by whoever holds
 * the `code_verifier`, which never leaves this tab until the exchange.
 */

// RFC 7636 §4.1: the verifier's alphabet is exactly these unreserved
// characters, and its length is 43..128.
const UNRESERVED = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~";

export function base64UrlEncode(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/**
 * A cryptographically random string of `length` characters over the PKCE
 * unreserved alphabet.
 *
 * Indexing the 66-character alphabet by a random byte is very slightly
 * biased (256 % 66 != 0); at these lengths that still leaves well over
 * 128 bits of entropy, which is what actually matters here.
 */
function randomUrlSafeString(length: number): string {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  let out = "";
  for (const byte of bytes) out += UNRESERVED[byte % UNRESERVED.length];
  return out;
}

/** A fresh `code_verifier`. 64 characters, comfortably inside 43..128. */
export function newVerifier(): string {
  return randomUrlSafeString(64);
}

/**
 * A fresh `state`. This is the CSRF defence on the callback: only a
 * redirect carrying the value this tab generated and stored is accepted,
 * so an attacker cannot feed us their own authorization code.
 */
export function newState(): string {
  return randomUrlSafeString(32);
}

/** The S256 challenge for a verifier: base64url(SHA-256(verifier)). */
export async function codeChallengeOf(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier));
  return base64UrlEncode(new Uint8Array(digest));
}
