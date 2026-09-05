import { useState } from "react";

export type Screen = "instances" | "users";

/** Which of the two screens is active. In-memory only — the mockup's own
 * navigation (goInstances/goUsers) isn't URL-addressed, and neither is
 * this: no client-side router, see the Admin UI design doc. */
export function useNav(initial: Screen = "instances") {
  const [screen, setScreen] = useState<Screen>(initial);
  return { screen, goInstances: () => setScreen("instances"), goUsers: () => setScreen("users") };
}
