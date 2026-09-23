import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useNav } from "./useNav";

const KEY = "mcplake:screen";

describe("useNav", () => {
  beforeEach(() => {
    localStorage.clear();
    // Tests below navigate for real via history.pushState; reset so one
    // test's URL can't leak into the next.
    window.history.pushState({}, "", "/");
  });

  it("defaults to the instances screen when nothing is stored", () => {
    const { result } = renderHook(() => useNav());
    expect(result.current.screen).toBe("instances");
  });

  it("restores the last screen from localStorage on mount -- a reload shouldn't reset navigation", () => {
    localStorage.setItem(KEY, "users");
    const { result } = renderHook(() => useNav());
    expect(result.current.screen).toBe("users");
  });

  it("persists to localStorage when navigating, so the next mount picks it up", () => {
    const { result } = renderHook(() => useNav());
    act(() => result.current.goUsers());
    expect(result.current.screen).toBe("users");
    expect(localStorage.getItem(KEY)).toBe("users");

    act(() => result.current.goInstances());
    expect(localStorage.getItem(KEY)).toBe("instances");
  });

  it("ignores a garbage stored value and falls back to the default", () => {
    localStorage.setItem(KEY, "not-a-real-screen");
    const { result } = renderHook(() => useNav());
    expect(result.current.screen).toBe("instances");
  });

  describe("URL awareness", () => {
    it("a direct load of /mcps/:name opens the instances screen even if users was remembered", () => {
      localStorage.setItem(KEY, "users");
      window.history.pushState({}, "", "/mcps/postgres-demo");

      const { result } = renderHook(() => useNav());

      expect(result.current.screen).toBe("instances");
    });

    it("a direct load of /users opens the users screen even if instances was remembered", () => {
      localStorage.setItem(KEY, "instances");
      window.history.pushState({}, "", "/users");

      const { result } = renderHook(() => useNav());

      expect(result.current.screen).toBe("users");
    });

    it("navigating pushes the corresponding path", () => {
      const { result } = renderHook(() => useNav());

      act(() => result.current.goUsers());
      expect(window.location.pathname).toBe("/users");

      act(() => result.current.goInstances());
      expect(window.location.pathname).toBe("/mcps");
    });

    it("browser back/forward between screens updates screen and the remembered default", () => {
      const { result } = renderHook(() => useNav());
      act(() => result.current.goUsers());

      act(() => {
        window.history.pushState({}, "", "/mcps");
        window.dispatchEvent(new PopStateEvent("popstate"));
      });

      expect(result.current.screen).toBe("instances");
      expect(localStorage.getItem(KEY)).toBe("instances");
    });

    it("ignores a popstate to an unrecognized path (e.g. bare '/') rather than clearing the screen", () => {
      const { result } = renderHook(() => useNav());
      act(() => result.current.goUsers());

      act(() => {
        window.history.pushState({}, "", "/");
        window.dispatchEvent(new PopStateEvent("popstate"));
      });

      expect(result.current.screen).toBe("users");
    });
  });

  describe("when localStorage throws (private browsing, quota, ...)", () => {
    beforeEach(() => {
      vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
        throw new Error("blocked");
      });
      vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
        throw new Error("blocked");
      });
    });
    afterEach(() => vi.restoreAllMocks());

    it("still works in-memory, just without persistence", () => {
      const { result } = renderHook(() => useNav());
      expect(result.current.screen).toBe("instances");
      act(() => result.current.goUsers());
      expect(result.current.screen).toBe("users");
    });
  });
});
