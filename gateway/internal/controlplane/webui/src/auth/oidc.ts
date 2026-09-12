import type { AuthConfig, LoginConfig } from "../api/types";
import { codeChallengeOf } from "./pkce";

/**
 * The OIDC Authorization Code + PKCE exchanges the admin UI performs
 * directly against the provider (ADR-0014). Nothing here talks to the
 * gateway: it hands out the provider's coordinates via
 * `GET /admin/auth/config` and then stays out of the way.
 *
 * Deliberately small and hand-written rather than a library: this is one
 * redirect and two form posts, and an air-gapped, `go:embed`-ed bundle
 * benefits from every dependency it doesn't ship.
 */

export type TokenResponse = {
  access_token: string;
  token_type?: string;
  expires_in?: number;
  refresh_token?: string;
  id_token?: string;
};

export type AuthorizeParams = { verifier: string; state: string; redirectUri: string };

/** Builds the URL to send the browser to, carrying the S256 challenge. */
export async function authorizeUrl(config: LoginConfig, { verifier, state, redirectUri }: AuthorizeParams): Promise<string> {
  // Parsed rather than string-concatenated so a provider whose endpoint
  // already carries query parameters (a tenant, an ACR hint) keeps them.
  const url = new URL(config.authorization_endpoint);
  const params = url.searchParams;
  params.set("response_type", "code");
  params.set("client_id", config.client_id);
  params.set("redirect_uri", redirectUri);
  params.set("scope", config.scopes.join(" "));
  params.set("state", state);
  params.set("code_challenge", await codeChallengeOf(verifier));
  params.set("code_challenge_method", "S256");
  return url.toString();
}

export type Callback =
  | { kind: "none" }
  | { kind: "code"; code: string; state: string | null }
  | { kind: "error"; message: string };

/**
 * Classifies the current URL: a returning authorization redirect, a
 * provider-side failure, or an ordinary page load.
 *
 * A `code` with no `state` is reported as a callback, not as "none", so
 * the caller rejects it explicitly instead of silently treating a
 * state-less (therefore unverifiable) redirect as a normal page load.
 */
export function readCallback(href: string): Callback {
  const params = new URL(href).searchParams;
  const error = params.get("error");
  if (error !== null) {
    return { kind: "error", message: params.get("error_description") ?? error };
  }
  const code = params.get("code");
  if (code !== null) {
    return { kind: "code", code, state: params.get("state") };
  }
  return { kind: "none" };
}

async function postForm(endpoint: string, body: URLSearchParams): Promise<TokenResponse> {
  const res = await fetch(endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString(),
  });

  let payload: Record<string, unknown> = {};
  try {
    payload = (await res.json()) as Record<string, unknown>;
  } catch {
    // Leave payload empty: the status check below still produces a
    // useful message, and a provider that answers non-JSON has already
    // told us enough.
  }

  if (!res.ok) {
    const description = (payload.error_description ?? payload.error) as string | undefined;
    throw new Error(description ?? `token endpoint returned ${res.status}`);
  }
  if (typeof payload.access_token !== "string") {
    // A 200 with no access_token usually means the client is registered
    // for the wrong flow (e.g. id_token only) -- say so rather than
    // storing `undefined` and failing much later with a confusing 401.
    throw new Error("token endpoint returned no access_token");
  }
  return payload as TokenResponse;
}

export type ExchangeParams = { code: string; verifier: string; redirectUri: string };

/** Redeems the authorization code. No client secret: public client. */
export function exchangeCode(config: LoginConfig, { code, verifier, redirectUri }: ExchangeParams): Promise<TokenResponse> {
  return postForm(
    config.token_endpoint,
    new URLSearchParams({
      grant_type: "authorization_code",
      code,
      redirect_uri: redirectUri,
      client_id: config.client_id,
      code_verifier: verifier,
    }),
  );
}

/** Trades a refresh token for a fresh access token, when one was issued. */
export function refreshWith(config: LoginConfig, refreshToken: string): Promise<TokenResponse> {
  return postForm(
    config.token_endpoint,
    new URLSearchParams({
      grant_type: "refresh_token",
      refresh_token: refreshToken,
      client_id: config.client_id,
    }),
  );
}

/**
 * Reads a JWT's payload **without verifying it**. Only ever used to show
 * the operator who they are signed in as; the gateway is the one that
 * verifies the token, and it re-checks the signature on every call.
 * Returns null for an opaque (non-JWT) access token, which is legitimate.
 */
export function decodeJwtPayload(token: string): Record<string, unknown> | null {
  const parts = token.split(".");
  if (parts.length !== 3) return null;
  try {
    const json = atob(parts[1].replace(/-/g, "+").replace(/_/g, "/"));
    const parsed: unknown = JSON.parse(json);
    return typeof parsed === "object" && parsed !== null && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

/** The most human-readable identity claim the token offers. */
export function subjectOf(token: string): string | null {
  const payload = decodeJwtPayload(token);
  if (!payload) return null;
  for (const claim of ["email", "preferred_username", "name", "sub"]) {
    if (typeof payload[claim] === "string" && payload[claim] !== "") return payload[claim];
  }
  return null;
}

/** Narrows the gateway's answer to the case where a login is possible. */
export function loginConfigOf(config: AuthConfig): LoginConfig | null {
  if (!config.client_id || !config.authorization_endpoint || !config.token_endpoint) return null;
  return {
    client_id: config.client_id,
    authorization_endpoint: config.authorization_endpoint,
    token_endpoint: config.token_endpoint,
    scopes: config.scopes ?? ["openid", "profile", "email"],
  };
}
