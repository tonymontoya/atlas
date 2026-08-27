import React from "react";
import { errorMessage } from "./format";

// The one implementation of the web app's load and submit choreography
// (deep-moded from the per-page copies). useResource owns abort-on-change,
// stale-result ignoring, and data retention across refetches; useMutation
// owns the double-submit guard and formatted write errors.

export type ResourceFetcher<T> = (signal: AbortSignal) => Promise<T>;

export type Resource<T> = {
  data: T | null;
  error: string | null;
  loading: boolean;
  reload: () => void;
};

export type Mutation<TArgs> = {
  run: (args: TArgs) => Promise<boolean>;
  busy: boolean;
  error: string | null;
};

// useResource loads whatever the fetcher fetches. A null fetcher means
// idle: no request, no data — the caller's "no selection" state. The
// deps array is the caller's refetch contract (useEffect-style); the
// fetcher's identity is deliberately not part of it, so inline closures
// do not cause refetch loops. Data from the last completed load is
// retained while the next one is in flight; reload() forces a refetch.
export function useResource<T>(
  fetch: ResourceFetcher<T> | null,
  deps: unknown[],
): Resource<T> {
  const [data, setData] = React.useState<T | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(false);
  const [reloadCount, setReloadCount] = React.useState(0);

  React.useEffect(() => {
    if (fetch === null) {
      setData(null);
      setError(null);
      setLoading(false);
      return;
    }
    const controller = new AbortController();
    let settled = false;
    setLoading(true);
    setError(null);
    fetch(controller.signal).then(
      (value) => {
        if (settled) {
          return;
        }
        setData(value);
        setLoading(false);
      },
      (reason: unknown) => {
        if (settled) {
          return;
        }
        setError(errorMessage(reason));
        setLoading(false);
      },
    );
    return () => {
      settled = true;
      controller.abort();
    };
    // The spread deps are the caller's refetch contract; the fetcher's
    // identity is intentionally excluded (see the doc comment above).
  }, [fetch === null, ...deps, reloadCount]);

  const reload = React.useCallback(() => {
    setReloadCount((count) => count + 1);
  }, []);

  return { data, error, loading, reload };
}

// useMutation wraps a write action. run resolves true on success and
// false on rejection (or on a double submit while one is in flight);
// the error arrives pre-formatted for display and is cleared at the
// start of each run. Success-side work stays at the call site.
export function useMutation<TArgs>(
  action: (args: TArgs) => Promise<unknown>,
): Mutation<TArgs> {
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const busyRef = React.useRef(false);

  const run = React.useCallback(
    async (args: TArgs): Promise<boolean> => {
      if (busyRef.current) {
        return false;
      }
      busyRef.current = true;
      setBusy(true);
      setError(null);
      try {
        await action(args);
        return true;
      } catch (reason: unknown) {
        setError(errorMessage(reason));
        return false;
      } finally {
        busyRef.current = false;
        setBusy(false);
      }
    },
    [action],
  );

  return { run, busy, error };
}
