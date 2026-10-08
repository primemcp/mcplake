import { describe, expect, it } from "vitest";
import { groupFilters, planGroupSave, stripGroupPrefix } from "./filterGroups";
import type { FilterPolicy } from "../api/types";

function policy(overrides: Partial<FilterPolicy>): FilterPolicy {
  return { name: "x", match: [], mcp: "local-fs", tool: "t", drop_fields: [], enabled: true, ...overrides };
}

describe("groupFilters", () => {
  it("groups filters whose name shares a '<group>::<tool>' prefix into one group", () => {
    const filters = [
      policy({ name: "hide-content::create_directory", tool: "create_directory" }),
      policy({ name: "hide-content::directory_tree", tool: "directory_tree" }),
      policy({ name: "unrelated", tool: "read_file" }),
    ];

    const groups = groupFilters(filters);

    expect(groups).toHaveLength(2);
    const hideContent = groups.find((g) => g.id === "hide-content")!;
    expect(hideContent.members.map((m) => m.tool).sort()).toEqual(["create_directory", "directory_tree"]);
    const unrelated = groups.find((g) => g.id === "unrelated")!;
    expect(unrelated.members).toEqual([filters[2]]);
  });

  it("treats a legacy plain name (no '::') as its own single-member group", () => {
    const filters = [policy({ name: "hide-pii", tool: "get_user" })];
    const groups = groupFilters(filters);
    expect(groups).toEqual([{ id: "hide-pii", members: [filters[0]] }]);
  });

  it("groups by everything before the FIRST '::' -- a name with an embedded separator merges into whatever group already owns that prefix", () => {
    // This is deliberate, useful behavior for a hand-crafted/legacy name --
    // but it's also exactly why ResponseFilterGroup rejects "::" in the
    // free-text name input at creation time: without that guard, an
    // operator naming a new filter "billing::extra" would silently produce
    // a member named "billing::extra::get_user", which groups under the
    // *existing* "billing" group's id, not a new one -- merging into (and,
    // on save, becoming editable/deletable alongside) an unrelated filter.
    const filters = [
      policy({ name: "billing::get_user", tool: "get_user" }),
      policy({ name: "billing::extra::list_users", tool: "list_users" }),
    ];
    const groups = groupFilters(filters);
    expect(groups).toHaveLength(1);
    expect(groups[0].id).toBe("billing");
    expect(groups[0].members).toHaveLength(2);
  });

  it("preserves insertion order of first appearance for group ids", () => {
    const filters = [
      policy({ name: "b::t1", tool: "t1" }),
      policy({ name: "a::t1", tool: "t1" }),
      policy({ name: "b::t2", tool: "t2" }),
    ];
    expect(groupFilters(filters).map((g) => g.id)).toEqual(["b", "a"]);
  });
});

describe("stripGroupPrefix", () => {
  it("strips a confirmed '<prefix>::' segment", () => {
    expect(stripGroupPrefix("alice::billing::get_user", "alice")).toBe("billing::get_user");
  });

  it("returns the name unchanged when it doesn't actually start with that prefix", () => {
    expect(stripGroupPrefix("bob::get_user", "alice")).toBe("bob::get_user");
  });
});

describe("planGroupSave", () => {
  it("creates a new suffixed member per tool when the group doesn't exist yet", () => {
    const plan = planGroupSave("hide-content", [], {
      create_directory: ["$.content"],
      directory_tree: ["$.content"],
    });

    expect(plan.toCreate.sort((a, b) => a.tool.localeCompare(b.tool))).toEqual([
      { name: "hide-content::create_directory", tool: "create_directory", dropFields: ["$.content"] },
      { name: "hide-content::directory_tree", tool: "directory_tree", dropFields: ["$.content"] },
    ]);
    expect(plan.toUpdate).toEqual([]);
    expect(plan.toDelete).toEqual([]);
  });

  it("updates an existing member in place (same name) when its tool is still desired", () => {
    const existing = [policy({ name: "hide-content::create_directory", tool: "create_directory", drop_fields: ["$.content"] })];

    const plan = planGroupSave("hide-content", existing, { create_directory: ["$.content", "$.other"] });

    expect(plan.toUpdate).toEqual([
      { name: "hide-content::create_directory", tool: "create_directory", dropFields: ["$.content", "$.other"] },
    ]);
    expect(plan.toCreate).toEqual([]);
    expect(plan.toDelete).toEqual([]);
  });

  it("deletes members for tools no longer present in the desired set", () => {
    const existing = [
      policy({ name: "hide-content::create_directory", tool: "create_directory" }),
      policy({ name: "hide-content::directory_tree", tool: "directory_tree" }),
    ];

    const plan = planGroupSave("hide-content", existing, { create_directory: ["$.content"] });

    expect(plan.toDelete).toEqual(["hide-content::directory_tree"]);
    expect(plan.toUpdate).toEqual([{ name: "hide-content::create_directory", tool: "create_directory", dropFields: ["$.content"] }]);
  });

  it("keeps a legacy bare name as-is when only that tool is edited (no rename)", () => {
    const existing = [policy({ name: "hide-pii", tool: "get_user", drop_fields: ["$.salary"] })];

    const plan = planGroupSave("hide-pii", existing, { get_user: ["$.salary", "$.ssn"] });

    expect(plan.toUpdate).toEqual([{ name: "hide-pii", tool: "get_user", dropFields: ["$.salary", "$.ssn"] }]);
    expect(plan.toCreate).toEqual([]);
  });

  it("adds a new suffixed member alongside an untouched legacy bare-name member", () => {
    const existing = [policy({ name: "hide-pii", tool: "get_user", drop_fields: ["$.salary"] })];

    const plan = planGroupSave("hide-pii", existing, {
      get_user: ["$.salary"],
      list_users: ["$.count"],
    });

    expect(plan.toUpdate).toEqual([{ name: "hide-pii", tool: "get_user", dropFields: ["$.salary"] }]);
    expect(plan.toCreate).toEqual([{ name: "hide-pii::list_users", tool: "list_users", dropFields: ["$.count"] }]);
  });

  it("does nothing for a tool whose desired field list is unchanged", () => {
    const existing = [policy({ name: "hide-content::create_directory", tool: "create_directory", drop_fields: ["$.content"] })];
    const plan = planGroupSave("hide-content", existing, { create_directory: ["$.content"] });
    expect(plan.toUpdate).toEqual([{ name: "hide-content::create_directory", tool: "create_directory", dropFields: ["$.content"] }]);
  });
});
