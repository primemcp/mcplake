import { describe, expect, it } from "vitest";
import { composeUsers, planUserSave, type UserFieldsByEndpoint } from "./users";
import type { AccessPolicy, FilterPolicy } from "./types";

function accessPolicy(overrides: Partial<AccessPolicy>): AccessPolicy {
  return { name: "alice", match: [], grants: [], ...overrides };
}

function filterPolicy(overrides: Partial<FilterPolicy>): FilterPolicy {
  return { name: "x", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: [], ...overrides };
}

describe("composeUsers", () => {
  it("composes a user with no filter policies yet -- access-only, per the design doc's explicit case", () => {
    const users = composeUsers([accessPolicy({ name: "alice" })], []);
    expect(users).toEqual([{ name: "alice", match: [], grants: [], filters: [] }]);
  });

  it("joins in every FilterPolicy whose name is grouped under the user's name", () => {
    const filters = [
      filterPolicy({ name: "alice::postgres-ro::get_user", mcp: "postgres-ro", tool: "get_user" }),
      filterPolicy({ name: "alice::postgres-ro::list_users", mcp: "postgres-ro", tool: "list_users" }),
      // A different user's filter must not leak in, even sharing a tool name.
      filterPolicy({ name: "bob::postgres-ro::get_user", mcp: "postgres-ro", tool: "get_user" }),
    ];
    const users = composeUsers([accessPolicy({ name: "alice" }), accessPolicy({ name: "bob" })], filters);

    expect(users.find((u) => u.name === "alice")!.filters).toHaveLength(2);
    expect(users.find((u) => u.name === "bob")!.filters).toHaveLength(1);
  });

  it("a filter policy whose name doesn't match any user's group id never joins to anyone", () => {
    const users = composeUsers([accessPolicy({ name: "alice" })], [filterPolicy({ name: "unrelated-thing" })]);
    expect(users[0].filters).toEqual([]);
  });

  it("a legacy bare-named filter policy (no '::') exactly matching a user's own name is its own single-member group -- same convention as lib/filterGroups", () => {
    const users = composeUsers([accessPolicy({ name: "alice" })], [filterPolicy({ name: "alice" })]);
    expect(users[0].filters).toHaveLength(1);
  });
});

describe("planUserSave", () => {
  const desired: UserFieldsByEndpoint = {
    "postgres-ro": { get_user: ["$.salary"], list_users: ["$.email"] },
  };

  it("creates a fresh user's AccessPolicy and one FilterPolicy per (mcp, tool) with dropped fields", () => {
    const plan = planUserSave("alice", null, { match: [], grants: [{ mcp: "postgres-ro", tools: ["*"] }] }, desired);

    expect(plan.accessPolicy).toEqual({
      name: "alice",
      match: [],
      grants: [{ mcp: "postgres-ro", tools: ["*"] }],
    });
    expect(plan.filtersToCreate.sort((a, b) => a.tool.localeCompare(b.tool))).toEqual([
      { name: "alice::postgres-ro::get_user", mcp: "postgres-ro", tool: "get_user", dropFields: ["$.salary"] },
      { name: "alice::postgres-ro::list_users", mcp: "postgres-ro", tool: "list_users", dropFields: ["$.email"] },
    ]);
    expect(plan.filtersToUpdate).toEqual([]);
    expect(plan.filtersToDelete).toEqual([]);
  });

  it("updates an existing member's fields in place (same name, no rename) when the tool is still desired", () => {
    const existing = {
      name: "alice",
      match: [],
      grants: [],
      filters: [
        filterPolicy({
          name: "alice::postgres-ro::get_user",
          mcp: "postgres-ro",
          tool: "get_user",
          drop_fields: ["$.salary"],
        }),
      ],
    };
    const plan = planUserSave("alice", existing, { match: [], grants: [] }, {
      "postgres-ro": { get_user: ["$.salary", "$.ssn"] },
    });

    expect(plan.filtersToUpdate).toEqual([
      { name: "alice::postgres-ro::get_user", mcp: "postgres-ro", tool: "get_user", dropFields: ["$.salary", "$.ssn"] },
    ]);
    expect(plan.filtersToCreate).toEqual([]);
    expect(plan.filtersToDelete).toEqual([]);
  });

  it("deletes members for (mcp, tool) pairs no longer present in the desired set", () => {
    const existing = {
      name: "alice",
      match: [],
      grants: [],
      filters: [
        filterPolicy({ name: "alice::postgres-ro::get_user", mcp: "postgres-ro", tool: "get_user" }),
        filterPolicy({ name: "alice::postgres-ro::list_users", mcp: "postgres-ro", tool: "list_users" }),
      ],
    };
    const plan = planUserSave("alice", existing, { match: [], grants: [] }, {
      "postgres-ro": { get_user: ["$.salary"] },
    });

    expect(plan.filtersToDelete).toEqual(["alice::postgres-ro::list_users"]);
  });

  it("keeps a legacy bare-name member (no '::') as-is when only that tool is edited -- no rename", () => {
    const existing = {
      name: "alice",
      match: [],
      grants: [],
      filters: [filterPolicy({ name: "alice", mcp: "postgres-ro", tool: "get_user", drop_fields: ["$.salary"] })],
    };
    const plan = planUserSave("alice", existing, { match: [], grants: [] }, {
      "postgres-ro": { get_user: ["$.salary"] },
    });

    expect(plan.filtersToUpdate).toEqual([
      { name: "alice", mcp: "postgres-ro", tool: "get_user", dropFields: ["$.salary"] },
    ]);
    expect(plan.filtersToCreate).toEqual([]);
  });
});
