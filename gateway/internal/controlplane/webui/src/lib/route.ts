/**
 * The admin UI's only URL-addressed state: which screen, and (for
 * "instances") which MCP is selected. Deliberately a few pure functions
 * over `window.location.pathname`/`history.pushState` rather than a router
 * library -- there are exactly two screens and one deep-linkable detail
 * page, so a dependency buys nothing a dozen lines of parsing doesn't
 * already cover (same reasoning as auth/oidc.ts's hand-rolled PKCE).
 *
 * Everything outside `/mcps` and `/users` (including bare "/") intentionally
 * resolves to `null`: those callers fall back to the remembered
 * localStorage screen (see state/useNav.ts), which is what every reload
 * that isn't a deep link should do.
 */

export type Route = { screen: "instances"; mcpName: string | null } | { screen: "users" };

export const MCPS_PATH = "/mcps";
export const USERS_PATH = "/users";

export function parseRoute(pathname: string): Route | null {
  const segments = pathname.split("/").filter(Boolean);
  if (segments[0] === "mcps") {
    return { screen: "instances", mcpName: segments[1] ? decodeURIComponent(segments[1]) : null };
  }
  if (segments[0] === "users") {
    return { screen: "users" };
  }
  return null;
}

/** The URL a selected MCP should live at. `name` is user-supplied (an
 * operator names their own MCPs), so it's encoded like any other path
 * segment carrying arbitrary text. */
export function mcpPath(name: string): string {
  return `${MCPS_PATH}/${encodeURIComponent(name)}`;
}
