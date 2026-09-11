import { describe, expect, it } from "vitest";
import type { AccessPolicy, FilterPolicy, MCPRegistration } from "../api/types";
import { simulate, type SimulationInput } from "./requestPath";

const memory: MCPRegistration = {
  name: "demo-memory",
  transport: "stdio",
  connect: { command: "memory" },
  status: "active",
  enabled: true,
  tools: {
    add_observations: { name: "add_observations" },
    delete_entities: { name: "delete_entities" },
  },
};

const analytics: MCPRegistration = {
  name: "analytics",
  transport: "stdio",
  connect: { command: "analytics" },
  status: "active",
  enabled: true,
  tools: { get_report: { name: "get_report" } },
};

const analyst: AccessPolicy = {
  name: "analyst-team",
  match: [{ path: "$.role", pattern: "^analyst$" }],
  grants: [{ mcp: "demo-memory", tools: ["add_observations"] }],
  enabled: true,
};

const payload = JSON.stringify({ sub: "u1", role: "analyst", groups: ["data"] });

function input(overrides: Partial<SimulationInput> = {}): SimulationInput {
  return {
    payload,
    mcp: "demo-memory",
    tool: "add_observations",
    endpoints: [memory, analytics],
    accessPolicies: [analyst],
    filterPolicies: [],
    ...overrides,
  };
}

function filter(overrides: Partial<FilterPolicy> & { name: string }): FilterPolicy {
  return {
    match: analyst.match,
    mcp: "demo-memory",
    tool: "add_observations",
    drop_fields: [],
    enabled: true,
    ...overrides,
  };
}

describe("simulate", () => {
  it("a matching, granted call passes every stage and reports the fields the matching filters drop", () => {
    const got = simulate(
      input({
        filterPolicies: [
          filter({ name: "analyst-team::pii::add_observations", drop_fields: ["$.results[*].email", "$.id"] }),
          filter({ name: "analyst-team::ids::add_observations", drop_fields: ["$.id", "$.owner"] }),
        ],
      }),
    );

    expect(got.outcome).toEqual({ kind: "ok", status: 200 });
    expect(got.authorized).toBe(true);
    // Union, deduplicated, in order of first appearance -- like
    // Engine.FieldsToRemove.
    expect(got.droppedFields).toEqual(["$.results[*].email", "$.id", "$.owner"]);
    expect(got.filters.map((f) => f.applied)).toEqual([true, true]);
  });

  it("only filters for the exact (mcp, tool) pair are considered at all", () => {
    const got = simulate(
      input({
        filterPolicies: [
          filter({ name: "other-tool", tool: "delete_entities", drop_fields: ["$.x"] }),
          filter({ name: "other-mcp", mcp: "analytics", drop_fields: ["$.y"] }),
          filter({ name: "this-one", drop_fields: ["$.z"] }),
        ],
      }),
    );

    expect(got.filters.map((f) => f.name)).toEqual(["this-one"]);
    expect(got.droppedFields).toEqual(["$.z"]);
  });

  it("a filter whose match fails, or which is disabled, is listed but not applied", () => {
    const got = simulate(
      input({
        filterPolicies: [
          filter({ name: "no-match", match: [{ path: "$.role", pattern: "^admin$" }], drop_fields: ["$.a"] }),
          filter({ name: "off", enabled: false, drop_fields: ["$.b"] }),
          filter({ name: "on", drop_fields: ["$.c"] }),
        ],
      }),
    );

    expect(got.filters.map((f) => [f.name, f.applied])).toEqual([
      ["no-match", false],
      ["off", false],
      ["on", true],
    ]);
    // A disabled filter is skipped before its match is even evaluated.
    expect(got.filters[1].match).toBeNull();
    expect(got.droppedFields).toEqual(["$.c"]);
  });

  it("reports every access policy's own match and grant decision, in order", () => {
    const admins: AccessPolicy = {
      name: "admins",
      match: [{ path: "$.role", pattern: "^admin$" }],
      grants: [{ mcp: "*", tools: ["*"] }],
      enabled: true,
    };
    const got = simulate(input({ accessPolicies: [admins, analyst] }));

    expect(got.policies.map((p) => [p.name, p.match?.matched, p.covers, p.grants])).toEqual([
      ["admins", false, true, false],
      ["analyst-team", true, true, true],
    ]);
  });

  it.each([
    ["a wildcard MCP grant", { mcp: "*", tools: ["add_observations"] }],
    ["a wildcard tools grant", { mcp: "demo-memory", tools: ["*"] }],
  ])("%s covers the call", (_label, grant) => {
    const got = simulate(input({ accessPolicies: [{ ...analyst, grants: [grant] }] }));

    expect(got.authorized).toBe(true);
  });

  it("a disabled access policy grants nothing and its match is never evaluated", () => {
    const got = simulate(input({ accessPolicies: [{ ...analyst, enabled: false }] }));

    expect(got.outcome).toEqual({ kind: "forbidden", status: 403, code: "forbidden" });
    expect(got.policies[0].match).toBeNull();
    expect(got.policies[0].grants).toBe(false);
  });

  it("a disabled policy doesn't suppress an enabled one that grants (policies are additive)", () => {
    const got = simulate(input({ accessPolicies: [{ ...analyst, name: "off", enabled: false }, analyst] }));

    expect(got.authorized).toBe(true);
  });

  it("claims that match no policy are forbidden, and no filter is evaluated", () => {
    const got = simulate(
      input({
        payload: JSON.stringify({ role: "guest" }),
        filterPolicies: [filter({ name: "f", match: [], drop_fields: ["$.a"] })],
      }),
    );

    expect(got.outcome).toEqual({ kind: "forbidden", status: 403, code: "forbidden" });
    expect(got.authorized).toBe(false);
    expect(got.filters[0].match).toBeNull();
    expect(got.filters[0].applied).toBe(false);
    expect(got.droppedFields).toEqual([]);
  });

  it("claims that match a policy without a grant for this (mcp, tool) are forbidden", () => {
    const got = simulate(input({ tool: "delete_entities" }));

    expect(got.outcome.kind).toBe("forbidden");
    expect(got.policies[0].match?.matched).toBe(true);
    expect(got.policies[0].covers).toBe(false);
  });

  it.each([
    ["not JSON at all", "{not json"],
    ["a JSON array", "[1, 2]"],
    ["a JSON scalar", '"just a string"'],
  ])("a payload that is %s is rejected at auth with 401, before any policy is looked at", (_label, bad) => {
    const got = simulate(input({ payload: bad }));

    expect(got.outcome.kind).toBe("invalid_payload");
    expect(got.outcome.status).toBe(401);
    expect(got.claims).toBeNull();
    expect(got.policies[0].match).toBeNull();
    expect(got.authorized).toBe(false);
  });

  it("an authorized call to a disabled endpoint is rejected with 403 mcp_disabled", () => {
    const got = simulate(input({ endpoints: [{ ...memory, enabled: false }] }));

    expect(got.outcome).toEqual({ kind: "mcp_disabled", status: 403, code: "mcp_disabled" });
    expect(got.authorized).toBe(true);
  });

  it("an unauthorized call to a disabled endpoint is plain forbidden -- the endpoint's state isn't disclosed", () => {
    const got = simulate(input({ payload: JSON.stringify({ role: "guest" }), endpoints: [{ ...memory, enabled: false }] }));

    expect(got.outcome.kind).toBe("forbidden");
  });

  it.each([
    ["an endpoint that isn't registered", { mcp: "nope" }],
    ["an endpoint that isn't active", { endpoints: [{ ...memory, status: "unreachable" }] }],
  ])("%s is 404 mcp_not_found once authorized", (_label, overrides) => {
    const got = simulate(
      input({
        accessPolicies: [{ ...analyst, grants: [{ mcp: "*", tools: ["*"] }] }],
        ...overrides,
      }),
    );

    expect(got.outcome).toEqual({ kind: "mcp_not_found", status: 404, code: "mcp_not_found" });
  });

  it("a tool the endpoint doesn't have is 404 tool_not_found once authorized", () => {
    const got = simulate(input({ accessPolicies: [{ ...analyst, grants: [{ mcp: "*", tools: ["*"] }] }], tool: "nope" }));

    expect(got.outcome).toEqual({ kind: "tool_not_found", status: 404, code: "tool_not_found" });
  });

  it("a policy whose rule can't be evaluated fails the request with 500, like the gateway's policy-evaluation error", () => {
    const got = simulate(input({ accessPolicies: [{ ...analyst, match: [{ path: "$.role", pattern: "(unclosed" }] }] }));

    expect(got.outcome.kind).toBe("policy_error");
    expect(got.outcome.status).toBe(500);
    expect(got.authorized).toBe(false);
  });

  it("a filter whose rule can't be evaluated also fails the request with 500", () => {
    const got = simulate(input({ filterPolicies: [filter({ name: "bad", match: [{ path: "$$", pattern: "x" }] })] }));

    expect(got.outcome.kind).toBe("policy_error");
    expect(got.authorized).toBe(true);
  });
});
