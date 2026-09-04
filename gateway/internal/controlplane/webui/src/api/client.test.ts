import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "./client";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("api.listMCPs", () => {
  it("returns the parsed list on success", async () => {
    const mcps = [{ name: "postgres-ro", transport: "stdio", connect: {}, status: "active" }];
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(mcps));
    vi.stubGlobal("fetch", fetchMock);

    const result = await api.listMCPs();

    expect(result).toEqual(mcps);
    expect(fetchMock).toHaveBeenCalledWith(
      "/admin/mcps",
      expect.objectContaining({ headers: expect.objectContaining({ "Content-Type": "application/json" }) }),
    );
  });

  it("throws ApiError with the parsed error body on failure", async () => {
    const errorBody = { error: "internal_error", message: "boom" };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse(errorBody, 500)));

    await expect(api.listMCPs()).rejects.toMatchObject(
      new ApiError(500, errorBody),
    );
  });
});

describe("api.registerMCP", () => {
  it("POSTs the request body and returns the created registration", async () => {
    const created = {
      name: "postgres-ro",
      transport: "stdio",
      connect: { command: "mcp-server-postgres" },
      status: "active",
    };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(created, 201));
    vi.stubGlobal("fetch", fetchMock);

    const result = await api.registerMCP({
      name: "postgres-ro",
      connect: { command: "mcp-server-postgres" },
    });

    expect(result).toEqual(created);
    const [, init] = fetchMock.mock.calls[0];
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({
      name: "postgres-ro",
      connect: { command: "mcp-server-postgres" },
    });
  });
});

describe("api.unregisterMCP", () => {
  it("resolves with no value on 204", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 204 })));

    await expect(api.unregisterMCP("postgres-ro")).resolves.toBeUndefined();
  });

  it("URL-encodes the name into the path", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await api.unregisterMCP("a/weird name");

    expect(fetchMock).toHaveBeenCalledWith(
      "/admin/mcps/a%2Fweird%20name",
      expect.anything(),
    );
  });
});

describe("network failure", () => {
  it("propagates the fetch rejection rather than throwing ApiError", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));

    await expect(api.listMCPs()).rejects.toThrow("Failed to fetch");
  });
});
