import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clearStorage, readStorage, writeStorage } from "./storage";

const KEY = "test:key";
const isString = (v: unknown): v is string => typeof v === "string";

describe("storage", () => {
  beforeEach(() => localStorage.clear());

  it("round-trips a value through write and read", () => {
    writeStorage(KEY, "hello");
    expect(readStorage(KEY, isString)).toBe("hello");
  });

  it("returns null when nothing is stored", () => {
    expect(readStorage(KEY, isString)).toBeNull();
  });

  it("returns null for a stored value that fails validation", () => {
    writeStorage(KEY, 42);
    expect(readStorage(KEY, isString)).toBeNull();
  });

  it("returns null for garbage (non-JSON) stored content", () => {
    localStorage.setItem(KEY, "{not json");
    expect(readStorage(KEY, isString)).toBeNull();
  });

  it("clearStorage removes the value", () => {
    writeStorage(KEY, "hello");
    clearStorage(KEY);
    expect(readStorage(KEY, isString)).toBeNull();
  });

  describe("when localStorage throws (private browsing, quota, ...)", () => {
    beforeEach(() => {
      vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
        throw new Error("blocked");
      });
      vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
        throw new Error("blocked");
      });
      vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
        throw new Error("blocked");
      });
    });
    afterEach(() => vi.restoreAllMocks());

    it("fails soft instead of throwing", () => {
      expect(() => writeStorage(KEY, "hello")).not.toThrow();
      expect(readStorage(KEY, isString)).toBeNull();
      expect(() => clearStorage(KEY)).not.toThrow();
    });
  });
});
