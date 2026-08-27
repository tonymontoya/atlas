// @vitest-environment jsdom
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useMutation, useResource } from "./resources";

// deferred gives tests manual control over a promise's settlement so
// loading/busy windows can be observed precisely.
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useResource", () => {
  it("stays idle when the fetcher is null", async () => {
    const { result } = renderHook(() => useResource<string>(null, []));

    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("loads data through the fetcher and clears loading", async () => {
    const fetcher = vi.fn().mockResolvedValue("clusters");
    const { result } = renderHook(() => useResource(fetcher, []));

    expect(result.current.loading).toBe(true);
    await act(async () => {});
    expect(result.current.data).toBe("clusters");
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
  });

  it("formats a rejected fetch as an error string", async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error("connection refused"));
    const { result } = renderHook(() => useResource(fetcher, []));

    await act(async () => {});
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBe("connection refused");
    expect(result.current.loading).toBe(false);
  });

  it("formats a non-Error rejection with the generic message", async () => {
    const fetcher = vi.fn().mockRejectedValue("nope");
    const { result } = renderHook(() => useResource(fetcher, []));

    await act(async () => {});
    expect(result.current.error).toBe("Request failed.");
  });

  it("clears a previous error on the next successful load", async () => {
    let fail = true;
    const fetcher = vi.fn(async () => {
      if (fail) {
        throw new Error("first load fails");
      }
      return "ok";
    });
    const { result } = renderHook(() => useResource(fetcher, []));

    await act(async () => {});
    expect(result.current.error).toBe("first load fails");

    fail = false;
    await act(async () => {
      result.current.reload();
    });
    expect(result.current.error).toBeNull();
    expect(result.current.data).toBe("ok");
  });

  it("retains the previous data and reports loading during a refetch", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const results = [first.promise, second.promise];
    const fetcher = vi.fn(() => results.shift() ?? Promise.resolve("done"));
    const { result, rerender } = renderHook(
      ({ key }) => useResource(fetcher, [key]),
      { initialProps: { key: "a" } },
    );

    await act(async () => {
      first.resolve("value-a");
    });
    expect(result.current.data).toBe("value-a");

    rerender({ key: "b" });
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBe("value-a");

    await act(async () => {
      second.resolve("value-b");
    });
    expect(result.current.loading).toBe(false);
    expect(result.current.data).toBe("value-b");
  });

  it("aborts an in-flight fetch when deps change and ignores its result", async () => {
    const stale = deferred<string>();
    const fresh = deferred<string>();
    const outcomes = [stale.promise, fresh.promise];
    const signals: AbortSignal[] = [];
    const fetcher = vi.fn((signal: AbortSignal) => {
      signals.push(signal);
      return outcomes.shift() ?? Promise.resolve("done");
    });
    const { result, rerender } = renderHook(
      ({ key }) => useResource(fetcher, [key]),
      { initialProps: { key: "a" } },
    );

    rerender({ key: "b" });
    expect(signals[0].aborted).toBe(true);
    expect(signals[1].aborted).toBe(false);

    // The stale fetch resolving after the abort must not clobber state.
    stale.resolve("stale");
    await act(async () => {});
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(true);

    fresh.resolve("fresh");
    await act(async () => {});
    expect(result.current.data).toBe("fresh");
    expect(result.current.loading).toBe(false);
  });

  it("aborts the in-flight fetch on unmount", async () => {
    const pending = deferred<string>();
    const signals: AbortSignal[] = [];
    const fetcher = vi.fn((signal: AbortSignal) => {
      signals.push(signal);
      return pending.promise;
    });
    const { unmount } = renderHook(() => useResource(fetcher, []));

    unmount();
    expect(signals[0].aborted).toBe(true);

    pending.resolve("late");
    await act(async () => {});
  });

  it("refetches on reload() with data retained until it lands", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    const results = [first.promise, second.promise];
    const fetcher = vi.fn(() => results.shift() ?? Promise.resolve("done"));
    const { result } = renderHook(() => useResource(fetcher, []));

    await act(async () => {
      first.resolve("page-one");
    });
    expect(result.current.data).toBe("page-one");

    let reload: () => void;
    await act(async () => {
      reload = result.current.reload;
      reload();
    });
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBe("page-one");

    await act(async () => {
      second.resolve("page-two");
    });
    expect(result.current.loading).toBe(false);
    expect(result.current.data).toBe("page-two");
  });
});

describe("useMutation", () => {
  it("resolves true and stays quiet when the action succeeds", async () => {
    const action = vi.fn().mockResolvedValue({ id: 1 });
    const { result } = renderHook(() => useMutation(action));

    let outcome = false;
    await act(async () => {
      outcome = await result.current.run({ title: "x" });
    });

    expect(outcome).toBe(true);
    expect(result.current.busy).toBe(false);
    expect(result.current.error).toBeNull();
    expect(action).toHaveBeenCalledWith({ title: "x" });
  });

  it("resolves false and formats the error when the action rejects", async () => {
    const action = vi.fn().mockRejectedValue(new Error("409 conflict"));
    const { result } = renderHook(() => useMutation(action));

    let outcome = true;
    await act(async () => {
      outcome = await result.current.run(undefined);
    });

    expect(outcome).toBe(false);
    expect(result.current.error).toBe("409 conflict");
    expect(result.current.busy).toBe(false);
  });

  it("guards double submits: a second run while busy resolves false without calling the action", async () => {
    const pending = deferred<number>();
    const action = vi.fn(() => pending.promise);
    const { result } = renderHook(() => useMutation(action));

    let first!: Promise<boolean>;
    await act(async () => {
      first = result.current.run(undefined);
    });
    expect(result.current.busy).toBe(true);

    let second = true;
    await act(async () => {
      second = await result.current.run(undefined);
    });
    expect(second).toBe(false);
    expect(action).toHaveBeenCalledTimes(1);

    await act(async () => {
      pending.resolve(1);
      expect(await first).toBe(true);
    });
    expect(result.current.busy).toBe(false);
  });

  it("clears the error at the start of the next run", async () => {
    const pending = deferred<string>();
    const rejectFirst = () => {
      throw new Error("boom");
    };
    const outcomes = [rejectFirst, () => pending.promise];
    const action = vi.fn(() => (outcomes.shift() ?? (() => pending.promise))());
    const { result } = renderHook(() => useMutation(action));

    let failed = true;
    await act(async () => {
      failed = await result.current.run(undefined);
    });
    expect(failed).toBe(false);
    expect(result.current.error).toBe("boom");

    let runPromise!: Promise<boolean>;
    await act(async () => {
      runPromise = result.current.run(undefined);
    });
    const observedDuringRun = result.current.error;
    await act(async () => {
      pending.resolve("ok");
      expect(await runPromise).toBe(true);
    });

    expect(observedDuringRun).toBeNull();
    expect(result.current.error).toBeNull();
  });
});
