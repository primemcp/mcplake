import { useCallback, useEffect, useRef, useState } from "react";

export type AsyncResource<T> = {
  data: T | null;
  loading: boolean;
  error: unknown;
  retry: () => Promise<void>;
};

/**
 * Shared shape behind every list/detail fetch in this app: load on mount,
 * expose {data, loading, error, retry}. Failure is an expected state a
 * screen section renders inline (see the Admin UI design doc's "Error
 * handling") — this hook never throws into the component tree.
 *
 * `loading` is true only until the *first* successful fetch -- every
 * consumer of `retry()` (every mutation hook's create/update/remove calls
 * this to refresh the list) is a background refresh, not a fresh page
 * load, and re-flipping `loading` to true would make callers that render
 * a "Loading…" placeholder blank out an already-populated list and pop
 * it back in on every single edit. That was a real, visible bug: toggling
 * one filter's enabled flag made the whole screen appear to jump, because
 * `UserList`'s entire list swapped for a loading string and back for the
 * ~100ms round trip. Once real data has ever arrived, a later retry keeps
 * showing it (even if that retry then fails -- `error` still surfaces
 * separately) instead of hiding it behind a loading state with nothing
 * useful to show in its place.
 */
export function useAsyncResource<T>(fetcher: () => Promise<T>, deps: unknown[]): AsyncResource<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const hasData = useRef(false);

  const load = useCallback(async () => {
    if (!hasData.current) setLoading(true);
    setError(null);
    try {
      const result = await fetcher();
      setData(result);
      hasData.current = true;
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
    // fetcher is expected to be re-created by the caller alongside deps.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  useEffect(() => {
    load();
  }, [load]);

  return { data, loading, error, retry: load };
}
