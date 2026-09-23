import { describe, expect, it } from "vitest";
import type { MCPRegistration } from "../api/types";
import {
  TRANSPORTS,
  asTransport,
  buildConnect,
  connectFieldsComplete,
  endpointTarget,
  errorField,
  isRemoteTransport,
} from "./transport";

describe("TRANSPORTS", () => {
  // These strings are a wire format (ADR-0017): persisted in the
  // registration store and accepted verbatim by the admin API. Re-spelling
  // one here would silently stop matching what the backend stores.
  it("matches mcp.SupportedTransports() exactly, in order", () => {
    expect(TRANSPORTS).toEqual(["stdio", "http", "sse"]);
  });
});

describe("isRemoteTransport", () => {
  it("treats http and sse as remote and stdio as local", () => {
    expect(isRemoteTransport("http")).toBe(true);
    expect(isRemoteTransport("sse")).toBe(true);
    expect(isRemoteTransport("stdio")).toBe(false);
  });
});

describe("asTransport", () => {
  it("passes through a known transport", () => {
    expect(asTransport("sse")).toBe("sse");
  });

  // A registration written by an older gateway carries "" (Register only
  // started normalizing it in ADR-0017), and the picker has to show
  // something rather than rendering nothing checked.
  it("falls back to stdio for an empty or unrecognized value", () => {
    expect(asTransport("")).toBe("stdio");
    expect(asTransport("carrier-pigeon")).toBe("stdio");
  });
});

describe("buildConnect", () => {
  const filled = { command: "mcp-server-postgres", args: "--read-only --db mcp", url: "https://x/mcp" };

  it("sends command and arguments for stdio, and no url", () => {
    expect(buildConnect("stdio", filled)).toEqual({
      command: "mcp-server-postgres",
      arguments: ["--read-only", "--db", "mcp"],
    });
  });

  it("sends url for a remote transport, and no command", () => {
    expect(buildConnect("http", filled)).toEqual({ url: "https://x/mcp" });
    expect(buildConnect("sse", filled)).toEqual({ url: "https://x/mcp" });
  });

  // Not cosmetic: config.validateMCP rejects a url on a stdio entry and a
  // command on an http one outright, so leaving the abandoned field in the
  // request turns a correctly filled form into a 400.
  it("never carries the other transport's field along", () => {
    expect(buildConnect("http", filled).command).toBeUndefined();
    expect(buildConnect("stdio", filled).url).toBeUndefined();
  });

  it("omits arguments entirely rather than sending an empty list", () => {
    expect(buildConnect("stdio", { ...filled, args: "   " }).arguments).toBeUndefined();
  });

  it("trims surrounding whitespace", () => {
    expect(buildConnect("http", { command: "", args: "", url: "  https://x/mcp  " })).toEqual({
      url: "https://x/mcp",
    });
  });
});

describe("connectFieldsComplete", () => {
  it("requires a command for stdio and a url for remote transports", () => {
    const cmdOnly = { command: "x", args: "", url: "" };
    const urlOnly = { command: "", args: "", url: "https://x/mcp" };

    expect(connectFieldsComplete("stdio", cmdOnly)).toBe(true);
    expect(connectFieldsComplete("http", cmdOnly)).toBe(false);
    expect(connectFieldsComplete("http", urlOnly)).toBe(true);
    expect(connectFieldsComplete("stdio", urlOnly)).toBe(false);
  });

  it("does not count whitespace as filled in", () => {
    expect(connectFieldsComplete("stdio", { command: "  ", args: "", url: "" })).toBe(false);
  });
});

describe("endpointTarget", () => {
  const base: MCPRegistration = {
    name: "x",
    transport: "stdio",
    connect: {},
    status: "active",
    enabled: true,
  };

  it("renders a stdio endpoint's command line", () => {
    expect(
      endpointTarget({ ...base, connect: { command: "bunx", arguments: ["-y", "server-fs"] } }),
    ).toBe("bunx -y server-fs");
  });

  it("renders a command with no arguments without a trailing space", () => {
    expect(endpointTarget({ ...base, connect: { command: "bunx" } })).toBe("bunx");
  });

  it("renders a remote endpoint's url", () => {
    expect(
      endpointTarget({ ...base, transport: "http", connect: { url: "https://x/mcp" } }),
    ).toBe("https://x/mcp");
  });

  it("returns an empty string rather than 'undefined' when nothing is set", () => {
    expect(endpointTarget(base)).toBe("");
    expect(endpointTarget({ ...base, transport: "sse" })).toBe("");
  });
});

describe("errorField", () => {
  it("attributes every shape of URL complaint mcp.NewClient produces to the url field", () => {
    const messages = [
      'mcp: Config.URL: must use https (got "http://mcp.example.com/mcp"); plaintext http is accepted only for a loopback host',
      "mcp: Config.URL is required for the http transport",
      'mcp: Config.URL: must be an absolute http(s) URL (got "/mcp")',
      'mcp: Config.URL: must include a host (got "https://")',
    ];
    for (const m of messages) expect(errorField(m)).toBe("url");
  });

  it("attributes a missing command to the command field", () => {
    expect(errorField("mcp: Config.Command is required for the stdio transport")).toBe("command");
  });

  // Anything it cannot place still reaches the operator, in the general
  // error slot -- misattributing would be worse than not attributing.
  it("returns null for a failure that is not about either field", () => {
    expect(errorField("adminservice: mcp registration failed: connection refused")).toBeNull();
    expect(errorField("name already registered")).toBeNull();
  });
});
