/**
 * Small localStorage wrapper shared by the screen-level "remember where the
 * operator was" state (selected user/tab, selected endpoint -- see
 * UsersScreen/InstancesScreen). Always fails soft: private browsing,
 * blocked storage, or a quota error should never block navigation or
 * selection, just skip persistence (same reasoning as useNav's original
 * try/catch, factored out now that a third caller needs it).
 */
export function readStorage<T>(key: string, isValid: (value: unknown) => value is T): T | null {
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return null;
    const parsed: unknown = JSON.parse(raw);
    return isValid(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

export function writeStorage(key: string, value: unknown): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // best effort
  }
}

export function clearStorage(key: string): void {
  try {
    localStorage.removeItem(key);
  } catch {
    // best effort
  }
}
