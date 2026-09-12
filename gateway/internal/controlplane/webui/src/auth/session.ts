import { clearStorage, readStorage, writeStorage } from "../lib/storage";
import type { TokenResponse } from "./oidc";

/**
 * Where the admin UI's token lives between renders and reloads.
 *
 * `sessionStorage`, per ADR-0014: scoped to this tab and gone when it
 * closes. `localStorage` would outlive the browsing session and be shared
 * with every other tab on the origin; memory-only would send the operator
 * back through the provider on every page reload, which for an
 * operational console is the difference between usable and not.
 */

const SESSION_KEY = "mcplake:auth:session";
const PENDING_KEY = "mcplake:auth:pending";

// Renew this far ahead of the stated expiry, so a request isn't lost to a
// token that expires between our check and the gateway's.
const EXPIRY_SKEW_MS = 5_000;

export type Session = {
  accessToken: string;
  refreshToken: string | null;
  /** Epoch ms, or null when the provider didn't state a lifetime. */
  expiresAt: number | null;
};

export type Pending = { verifier: string; state: string };

function isSession(v: unknown): v is Session {
  if (typeof v !== "object" || v === null) return false;
  const s = v as Record<string, unknown>;
  return (
    typeof s.accessToken === "string" &&
    (s.refreshToken === null || typeof s.refreshToken === "string") &&
    (s.expiresAt === null || typeof s.expiresAt === "number")
  );
}

function isPending(v: unknown): v is Pending {
  if (typeof v !== "object" || v === null) return false;
  const p = v as Record<string, unknown>;
  return typeof p.verifier === "string" && typeof p.state === "string";
}

/**
 * Builds a session from a token response. `previousRefreshToken` covers
 * the refresh case: providers commonly omit `refresh_token` when they are
 * not rotating it, and dropping it would silently downgrade the session
 * to one that can't be renewed again.
 */
export function sessionFrom(tokens: TokenResponse, now: number, previousRefreshToken: string | null = null): Session {
  return {
    accessToken: tokens.access_token,
    refreshToken: tokens.refresh_token ?? previousRefreshToken,
    expiresAt: typeof tokens.expires_in === "number" ? now + tokens.expires_in * 1000 : null,
  };
}

export function isExpired(session: Session, now: number): boolean {
  // No stated lifetime means we cannot know; let the gateway's 401 be the
  // judge rather than pre-emptively discarding a working token.
  if (session.expiresAt === null) return false;
  return now >= session.expiresAt - EXPIRY_SKEW_MS;
}

export function loadSession(): Session | null {
  return readStorage(SESSION_KEY, isSession, sessionStorage);
}

export function saveSession(session: Session): void {
  writeStorage(SESSION_KEY, session, sessionStorage);
}

export function clearSession(): void {
  clearStorage(SESSION_KEY, sessionStorage);
}

export function savePending(pending: Pending): void {
  writeStorage(PENDING_KEY, pending, sessionStorage);
}

/**
 * Reads and removes the pending PKCE state. Single-use on purpose: a
 * callback replayed against an already-consumed state finds nothing and
 * is rejected.
 */
export function takePending(): Pending | null {
  const pending = readStorage(PENDING_KEY, isPending, sessionStorage);
  clearStorage(PENDING_KEY, sessionStorage);
  return pending;
}
