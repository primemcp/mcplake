import { describe, expect, it } from "vitest";
import { JSONPathError, selectPath } from "./jsonpath";

// The same document router/claimrule_test.go evaluates its rules against,
// so the two suites can be compared case by case.
const doc = {
  role: "db-writer",
  sub: "user-123",
  groups: ["oncall-db", "team-data-eng"],
  empty_groups: [],
  "https://example.com/org": { tier: "enterprise", region: null },
  id: 42,
  nested: { a: { tier: "gold" }, b: [{ tier: "silver" }, { tier: "bronze" }] },
};

describe("selectPath", () => {
  it.each<[string, string, unknown[]]>([
    ["the root alone", "$", [doc]],
    ["a top-level member", "$.role", ["db-writer"]],
    ["a member holding an array (the whole array, not its elements)", "$.groups", [["oncall-db", "team-data-eng"]]],
    ["array elements via [*]", "$.groups[*]", ["oncall-db", "team-data-eng"]],
    ["a positive index", "$.groups[0]", ["oncall-db"]],
    ["a negative index counts from the end", "$.groups[-1]", ["team-data-eng"]],
    ["an out-of-range index selects nothing", "$.groups[5]", []],
    ["a slice", "$.groups[0:1]", ["oncall-db"]],
    ["a bracketed double-quoted name (namespaced claim)", '$["https://example.com/org"].tier', ["enterprise"]],
    ["a bracketed single-quoted name", "$['https://example.com/org'].tier", ["enterprise"]],
    ["a null-valued member (present, so selected)", '$["https://example.com/org"].region', [null]],
    ["a numeric member", "$.id", [42]],
    ["an object wildcard selects member values", "$.nested.a.*", ["gold"]],
    ["a bracket wildcard on an object selects member values", "$.nested.a[*]", ["gold"]],
    ["a union of names", "$.nested.b[0]['tier','missing']", ["silver"]],
    ["descendant search", "$..tier", ["enterprise", "gold", "silver", "bronze"]],
    ["descendant search with a bracket selector", "$.nested..[0]", [{ tier: "silver" }]],
    ["a missing member selects nothing (not an error)", "$.nonexistent", []],
    ["a member under a missing member selects nothing", "$.nonexistent.deeper", []],
    ["a member under a scalar selects nothing", "$.role.deeper", []],
    ["a member under null selects nothing", '$["https://example.com/org"].region.deeper', []],
    ["an index on an object selects nothing", "$.nested[0]", []],
    ["a name on an array selects nothing", "$.groups.name", []],
    ["whitespace inside brackets is allowed", "$[ 'role' ]", ["db-writer"]],
  ])("selects %s", (_label, path, want) => {
    expect(selectPath(doc, path)).toEqual(want);
  });

  it("returns nothing for any path against a scalar document", () => {
    expect(selectPath("just a string", "$.role")).toEqual([]);
    expect(selectPath(null, "$.role")).toEqual([]);
  });

  it.each<[string, string]>([
    ["a path that doesn't start with $", "role"],
    ["an empty path", ""],
    ["a trailing dot", "$."],
    ["an unterminated bracket", "$.groups["],
    ["an unterminated quoted name", "$['role"],
    ["an empty bracket", "$[]"],
    ["a filter selector (unsupported here, see the module doc comment)", "$.groups[?(@ == 'x')]"],
    ["garbage after a complete path", "$.role $$"],
  ])("throws JSONPathError for %s", (_label, path) => {
    expect(() => selectPath(doc, path)).toThrow(JSONPathError);
  });
});
