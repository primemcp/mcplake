/**
 * Small localStorage wrapper shared by the screen-level "remember where the
 * operator was" state (selected user/tab, selected endpoint -- see
 * UsersScreen/InstancesScreen). Always fails soft: private browsing,
 * blocked storage, or a quota error should never block navigation or
 * selection, just skip persistence (same reasoning as useNav's original
 * try/catch, factored out now that a third caller needs it).
 *
 * `area` selects which Storage to use. It defaults to localStorage, which
 * is what the "remember where the operator was" callers want; the auth
 * session passes sessionStorage instead, so the token dies with the tab
 * (see auth/session.ts and ADR-0014).
 */
export function readStorage<T>(
  key: string,
  isValid: (value: unknown) => value is T,
  area: Storage = localStorage,
): T | null {
  try {
    const raw = area.getItem(key);
    if (raw === null) return null;
    const parsed: unknown = JSON.parse(raw);
    return isValid(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export function writeStorage(key: string, value: unknown, area: Storage = localStorage): void {
  try {
    area.setItem(key, JSON.stringify(value));
  } catch {
    // best effort
  }
}

export function clearStorage(key: string, area: Storage = localStorage): void {
  try {
    area.removeItem(key);
  } catch {
    // best effort
  }
}
