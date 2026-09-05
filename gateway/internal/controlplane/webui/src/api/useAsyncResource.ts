import { useCallback, useEffect, useState } from "react";

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
 */
export function useAsyncResource<T>(fetcher: () => Promise<T>, deps: unknown[]): AsyncResource<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setData(await fetcher());
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
