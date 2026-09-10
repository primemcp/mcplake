import { describe, expect, it } from "vitest";
import { composeUsers } from "./users";
import type { AccessPolicy, FilterPolicy } from "./types";

function accessPolicy(overrides: Partial<AccessPolicy>): AccessPolicy {
  return { name: "alice", match: [], grants: [], enabled: true, ...overrides };
}

function filterPolicy(overrides: Partial<FilterPolicy>): FilterPolicy {
  return { name: "x", match: [], mcp: "postgres-ro", tool: "get_user", drop_fields: [], enabled: true, ...overrides };
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
