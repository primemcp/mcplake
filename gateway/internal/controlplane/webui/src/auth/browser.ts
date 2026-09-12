/**
 * The three things the login flow does to the browser itself, behind one
 * seam. They are the only genuinely untestable parts of the flow (jsdom
 * has no navigation), so keeping them here lets AuthProvider's own tests
 * drive a real state machine against a fake browser instead of mocking
 * the state machine itself.
 */

export function currentHref(): string {
  return window.location.href;
}

/** The OAuth `redirect_uri`: always this app's own origin. Never taken
 * from a query parameter — that is how open redirects happen. It must
 * match byte-for-byte between the authorize request and the token
 * exchange, so both callers go through here. */
export function redirectUri(): string {
  return `${window.location.origin}/`;
}

export function redirectTo(url: string): void {
  window.location.assign(url);
}

/** Drops `?code=…&state=…` once consumed, so a reload (or a shared URL)
 * can't replay a spent authorization code. */
export function stripQuery(): void {
  window.history.replaceState({}, "", window.location.pathname);
}
