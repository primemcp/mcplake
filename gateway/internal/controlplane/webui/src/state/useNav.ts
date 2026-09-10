import { useState } from "react";

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

/**
 * Which of the two screens is active, persisted to localStorage so a page
 * reload lands back where the operator left off instead of always
 * bouncing to MCP connections. Still no client-side router (the mockup's
 * own navigation isn't URL-addressed either, see the Admin UI design
 * doc) -- this is just a remembered default, not a real route.
 */
export function useNav(initial: Screen = "instances") {
  const [screen, setScreen] = useState<Screen>(() => readStoredScreen() ?? initial);

  const go = (next: Screen) => {
    setScreen(next);
    writeStoredScreen(next);
  };

  return { screen, goInstances: () => go("instances"), goUsers: () => go("users") };
}
