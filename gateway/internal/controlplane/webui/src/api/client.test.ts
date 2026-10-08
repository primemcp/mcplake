import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError, configureAuth } from "./client";

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

describe("authenticated requests", () => {
  afterEach(() => {
    configureAuth(null);
  });

  it("attaches the bearer token the auth layer provides", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse([]));
    vi.stubGlobal("fetch", fetchMock);
    configureAuth({ getToken: () => Promise.resolve("tok-123"), onAuthFailure: vi.fn() });

    await api.listMCPs();

    expect(fetchMock).toHaveBeenCalledWith(
      "/admin/mcps",
      expect.objectContaining({ headers: expect.objectContaining({ Authorization: "Bearer tok-123" }) }),
    );
  });

  // With the gate off there is no token and no header -- the admin API
  // must keep working exactly as it did before ADR-0010.
  it("sends no Authorization header when there is no token", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse([]));
    vi.stubGlobal("fetch", fetchMock);
    configureAuth({ getToken: () => Promise.resolve(null), onAuthFailure: vi.fn() });

    await api.listMCPs();

    expect(fetchMock.mock.calls[0][1].headers).not.toHaveProperty("Authorization");
  });

  it.each([401, 403])("reports a %d to the auth layer, and still throws so the caller sees it", async (status) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ error: "nope", message: "nope" }, status)));
    const onAuthFailure = vi.fn();
    configureAuth({ getToken: () => Promise.resolve("tok"), onAuthFailure });

    await expect(api.listMCPs()).rejects.toBeInstanceOf(ApiError);

    expect(onAuthFailure).toHaveBeenCalledWith(status);
  });

  it("leaves other failures alone", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ error: "boom", message: "boom" }, 500)));
    const onAuthFailure = vi.fn();
    configureAuth({ getToken: () => Promise.resolve("tok"), onAuthFailure });

    await expect(api.listMCPs()).rejects.toBeInstanceOf(ApiError);

    expect(onAuthFailure).not.toHaveBeenCalled();
  });
});

describe("api.authConfig", () => {
  it("reads the unauthenticated bootstrap endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ auth_required: false }));
    vi.stubGlobal("fetch", fetchMock);

    const config = await api.authConfig();

    expect(config).toEqual({ auth_required: false });
    expect(fetchMock.mock.calls[0][0]).toBe("/admin/auth/config");
  });
});
