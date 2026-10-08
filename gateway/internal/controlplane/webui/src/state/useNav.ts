import { useEffect, useState } from "react";
import { MCPS_PATH, USERS_PATH, parseRoute } from "../lib/route";

export type Screen = "instances" | "users";

const STORAGE_KEY = "mcplake:screen";

function isScreen(value: unknown): value is Screen {
  return value === "instances" || value === "users";
}

function readStoredScreen(): Screen | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return isScreen(stored) ? stored : null;
  } catch {
    // Private browsing, disabled storage, etc. -- navigation still works,
    // it just won't survive a reload.
    return null;
  }
}

function writeStoredScreen(screen: Screen) {
  try {
    localStorage.setItem(STORAGE_KEY, screen);
  } catch {
    // Same as above -- best effort, never block navigation on this.
  }
}

function pathFor(screen: Screen): string {
  return screen === "instances" ? MCPS_PATH : USERS_PATH;
}

/**
 * Which of the two screens is active. A direct load of a recognized URL
 * (`/mcps`, `/mcps/:name`, `/users`) wins over the remembered screen -- a
 * deep link should open what it links to, not wherever the operator was
 * last -- and browser back/forward between two different screens is
 * handled via `popstate`. Anything else (including bare "/") falls back to
 * localStorage, so a plain reload still lands where the operator left off.
 *
 * This owns the screen, not the selected MCP: InstancesScreen already
 * persists+validates that against the live endpoint list, and duplicating
 * it here would just be two sources of truth for the same thing. It reads
 * the same URL (see `parseRoute`) to seed its own initial selection.
 */
export function useNav(initial: Screen = "instances") {
  const [screen, setScreen] = useState<Screen>(() => parseRoute(window.location.pathname)?.screen ?? readStoredScreen() ?? initial);

  const go = (next: Screen) => {
    setScreen(next);
    writeStoredScreen(next);
    window.history.pushState(null, "", pathFor(next));
  };

  useEffect(() => {
    const onPopState = () => {
      const route = parseRoute(window.location.pathname);
      if (!route) return;
      setScreen(route.screen);
      writeStoredScreen(route.screen);
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  return { screen, goInstances: () => go("instances"), goUsers: () => go("users") };
}
