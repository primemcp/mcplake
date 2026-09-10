import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useAsyncResource } from "./useAsyncResource";

describe("useAsyncResource", () => {
  it("is loading on the initial fetch, then not once data arrives", async () => {
    const fetcher = vi.fn().mockResolvedValue("first");
    const { result } = renderHook(() => useAsyncResource(fetcher, []));

    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBe("first");
  });

  it("a background retry after data already exists never flips loading back to true -- avoids blanking the UI on every refresh", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce("first").mockResolvedValueOnce("second");
    const { result } = renderHook(() => useAsyncResource(fetcher, []));
    await waitFor(() => expect(result.current.data).toBe("first"));

    let retryPromise!: Promise<void>;
    act(() => {
      retryPromise = result.current.retry();
    });
    // Still showing the old data, and never reports loading, while the
    // second fetch is in flight.
    expect(result.current.loading).toBe(false);
    expect(result.current.data).toBe("first");

    await act(() => retryPromise);
    expect(result.current.data).toBe("second");
    expect(result.current.loading).toBe(false);
  });

  it("a retry after the initial load failed (still no data) shows loading again -- there's nothing to keep displaying", async () => {
    const fetcher = vi.fn().mockRejectedValueOnce(new Error("boom")).mockResolvedValueOnce("ok");
    const { result } = renderHook(() => useAsyncResource(fetcher, []));
    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.loading).toBe(false);
    expect(result.current.data).toBe(null);

    let retryPromise!: Promise<void>;
    act(() => {
      retryPromise = result.current.retry();
    });
    expect(result.current.loading).toBe(true);

    await act(() => retryPromise);
    expect(result.current.data).toBe("ok");
    expect(result.current.loading).toBe(false);
  });

  it("a background retry that fails keeps the last-known-good data on screen, surfaced only via error", async () => {
    const fetcher = vi.fn().mockResolvedValueOnce("first").mockRejectedValueOnce(new Error("boom"));
    const { result } = renderHook(() => useAsyncResource(fetcher, []));
    await waitFor(() => expect(result.current.data).toBe("first"));

    await act(() => result.current.retry());

    expect(result.current.data).toBe("first");
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeTruthy();
  });
});
