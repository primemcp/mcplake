import { describe, expect, it } from "vitest";
import { MCPS_PATH, USERS_PATH, mcpPath, parseRoute } from "./route";

describe("parseRoute", () => {
  it("reads the instances screen with a selected MCP from /mcps/:name", () => {
    expect(parseRoute("/mcps/postgres-demo")).toEqual({ screen: "instances", mcpName: "postgres-demo" });
  });

  it("reads the instances screen with no selection from bare /mcps", () => {
    expect(parseRoute("/mcps")).toEqual({ screen: "instances", mcpName: null });
    expect(parseRoute("/mcps/")).toEqual({ screen: "instances", mcpName: null });
  });

  it("decodes a name that needed encoding", () => {
    expect(parseRoute("/mcps/my%20mcp")).toEqual({ screen: "instances", mcpName: "my mcp" });
  });

  it("reads the users screen", () => {
    expect(parseRoute("/users")).toEqual({ screen: "users" });
  });

  it("returns null for the root and anything unrecognized -- callers fall back to the remembered screen", () => {
    expect(parseRoute("/")).toBeNull();
    expect(parseRoute("")).toBeNull();
    expect(parseRoute("/something-else")).toBeNull();
  });
});

describe("mcpPath", () => {
  it("builds a path under MCPS_PATH", () => {
    expect(mcpPath("postgres-demo")).toBe(`${MCPS_PATH}/postgres-demo`);
  });

  it("encodes characters that aren't safe bare in a path segment", () => {
    expect(mcpPath("my mcp")).toBe(`${MCPS_PATH}/my%20mcp`);
  });

  it("round-trips through parseRoute", () => {
    expect(parseRoute(mcpPath("weird/name?x"))).toEqual({ screen: "instances", mcpName: "weird/name?x" });
  });
});

it("USERS_PATH parses back to the users screen", () => {
  expect(parseRoute(USERS_PATH)).toEqual({ screen: "users" });
});
