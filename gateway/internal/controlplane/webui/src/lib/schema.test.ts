import { describe, expect, it } from "vitest";
import {
  annotateFields,
  buildGraph,
  collapseFields,
  countDroppedDescendants,
  effectiveDropped,
  enablePath,
  flattenSchema,
} from "./schema";

describe("flattenSchema", () => {
  it("flattens top-level primitive fields", () => {
    const rows = flattenSchema({ type: "object", properties: { id: { type: "string" } } });
    expect(rows).toEqual([{ path: "$.id", type: "string", indent: 0, description: undefined }]);
  });

  it("recurses into a nested object", () => {
    const rows = flattenSchema({
      type: "object",
      properties: { meta: { type: "object", properties: { count: { type: "number" } } } },
    });
    expect(rows.map((r) => [r.path, r.type, r.indent])).toEqual([
      ["$.meta", "object", 0],
      ["$.meta.count", "number", 1],
    ]);
  });

  it("labels an array of primitives as a leaf, without expanding it", () => {
    const rows = flattenSchema({
      type: "object",
      properties: { tags: { type: "array", items: { type: "string" } } },
    });
    expect(rows).toEqual([{ path: "$.tags", type: "array<string>", indent: 0, description: undefined }]);
  });

  it("describes an array of arrays recursively", () => {
    const rows = flattenSchema({
      type: "object",
      properties: { matrix: { type: "array", items: { type: "array", items: { type: "number" } } } },
    });
    expect(rows[0].type).toBe("array<array<number>>");
  });

  it("expands an array of objects using the [*] wildcard, matching real backend JSONPath syntax", () => {
    // The exact shape of @modelcontextprotocol/server-memory's create_entities,
    // registered and inspected against the real running gateway.
    const rows = flattenSchema({
      type: "object",
      properties: {
        entities: {
          type: "array",
          items: {
            type: "object",
            properties: {
              name: { type: "string", description: "The name of the entity" },
              entityType: { type: "string" },
              observations: { type: "array", items: { type: "string" } },
            },
          },
        },
      },
    });

    expect(rows.map((r) => [r.path, r.type, r.indent])).toEqual([
      ["$.entities", "array<object>", 0],
      ["$.entities[*].name", "string", 1],
      ["$.entities[*].entityType", "string", 1],
      ["$.entities[*].observations", "array<string>", 1],
    ]);
    expect(rows.find((r) => r.path === "$.entities[*].name")?.description).toBe("The name of the entity");
  });

  it("recurses to arbitrary depth: object -> array<object> -> array<object> -> field", () => {
    const rows = flattenSchema({
      type: "object",
      properties: {
        teams: {
          type: "array",
          items: {
            type: "object",
            properties: {
              members: {
                type: "array",
                items: { type: "object", properties: { email: { type: "string" } } },
              },
            },
          },
        },
      },
    });

    const paths = rows.map((r) => r.path);
    expect(paths).toContain("$.teams");
    expect(paths).toContain("$.teams[*].members");
    expect(paths).toContain("$.teams[*].members[*].email");

    const emailRow = rows.find((r) => r.path === "$.teams[*].members[*].email");
    expect(emailRow?.indent).toBe(2);
  });

  it("returns an empty list for a schema with no properties", () => {
    expect(flattenSchema(undefined)).toEqual([]);
    expect(flattenSchema({ type: "object" })).toEqual([]);
  });
});

const DEEP_SCHEMA = {
  type: "object",
  properties: {
    entities: {
      type: "array",
      items: {
        type: "object",
        properties: {
          name: { type: "string" },
          tags: { type: "array", items: { type: "string" } },
        },
      },
    },
    id: { type: "string" },
  },
};

describe("annotateFields", () => {
  it("marks a leaf as having no children and zero descendants", () => {
    const [entities, name, tags, id] = annotateFields(flattenSchema(DEEP_SCHEMA));
    expect(entities.hasChildren).toBe(true);
    expect(entities.descendantCount).toBe(2); // name and tags, both direct children
    expect(name.hasChildren).toBe(false);
    expect(name.descendantCount).toBe(0);
    expect(tags.hasChildren).toBe(false); // array<string> is a leaf, not expandable
    expect(id.hasChildren).toBe(false);
  });

  it("counts every descendant recursively, not just direct children", () => {
    // object -> array<object> -> array<object> -> leaf: "a" has 2 fields
    // living under it (b and c), even though only b is a direct child.
    const rows = flattenSchema({
      type: "object",
      properties: {
        a: {
          type: "array",
          items: {
            type: "object",
            properties: { b: { type: "array", items: { type: "object", properties: { c: { type: "string" } } } } },
          },
        },
      },
    });
    const [a] = annotateFields(rows);
    expect(a.descendantCount).toBe(2);
  });
});

describe("collapseFields", () => {
  it("hides a node's descendants by default (nothing expanded)", () => {
    const annotated = annotateFields(flattenSchema(DEEP_SCHEMA));
    const visible = collapseFields(annotated, () => false, false);
    expect(visible.map((r) => r.path)).toEqual(["$.entities", "$.id"]);
  });

  it("reveals a node's direct children once it's marked expanded", () => {
    const annotated = annotateFields(flattenSchema(DEEP_SCHEMA));
    const visible = collapseFields(annotated, (r) => r.path === "$.entities", false);
    expect(visible.map((r) => r.path)).toEqual([
      "$.entities",
      "$.entities[*].name",
      "$.entities[*].tags",
      "$.id",
    ]);
  });

  it("ignores expand state entirely while searching -- everything shows", () => {
    const annotated = annotateFields(flattenSchema(DEEP_SCHEMA));
    const visible = collapseFields(annotated, () => false, true);
    expect(visible).toHaveLength(annotated.length);
  });
});

describe("buildGraph", () => {
  it("builds one node per field and one edge per parent-child link", () => {
    const { nodes, edges } = buildGraph(flattenSchema(DEEP_SCHEMA));

    expect(nodes.map((n) => n.id).sort()).toEqual(["$.entities", "$.entities[*].name", "$.entities[*].tags", "$.id"].sort());
    expect(edges).toEqual([
      { id: "$.entities->$.entities[*].name", source: "$.entities", target: "$.entities[*].name" },
      { id: "$.entities->$.entities[*].tags", source: "$.entities", target: "$.entities[*].tags" },
    ]);
    // $.id is a separate top-level root -- no edge connects it to anything.
    expect(edges.some((e) => e.target === "$.id" || e.source === "$.id")).toBe(false);
  });

  it("labels a node with just its final path segment, stripping [*] wildcards", () => {
    const { nodes } = buildGraph(flattenSchema(DEEP_SCHEMA));
    const nameNode = nodes.find((n) => n.id === "$.entities[*].name");
    expect(nameNode?.label).toBe("name");
  });

  it("correctly parents nodes across two independent top-level trees, not just one", () => {
    const rows = flattenSchema({
      type: "object",
      properties: {
        a: { type: "object", properties: { x: { type: "string" } } },
        b: { type: "object", properties: { y: { type: "string" } } },
      },
    });
    const { edges } = buildGraph(rows);
    expect(edges).toEqual([
      { id: "$.a->$.a.x", source: "$.a", target: "$.a.x" },
      { id: "$.b->$.b.y", source: "$.b", target: "$.b.y" },
    ]);
  });
});

describe("countDroppedDescendants", () => {
  it("counts zero for every row when nothing is dropped", () => {
    const rows = flattenSchema(DEEP_SCHEMA);
    expect(countDroppedDescendants(rows, () => false)).toEqual([0, 0, 0, 0]);
  });

  it("counts only dropped descendants, not the row itself or its siblings", () => {
    const rows = flattenSchema(DEEP_SCHEMA); // entities, entities[*].name, entities[*].tags, id
    const dropped = new Set(["$.entities[*].name"]);
    const counts = countDroppedDescendants(rows, (r) => dropped.has(r.path));
    expect(counts).toEqual([1, 0, 0, 0]); // entities has 1 dropped descendant; id has none (not a descendant)
  });

  it("counts transitively, including grandchildren", () => {
    const rows = flattenSchema({
      type: "object",
      properties: {
        a: {
          type: "array",
          items: {
            type: "object",
            properties: { b: { type: "array", items: { type: "object", properties: { c: { type: "string" } } } } },
          },
        },
      },
    });
    const dropped = new Set(["$.a[*].b[*].c"]);
    const [a, b, c] = countDroppedDescendants(rows, (r) => dropped.has(r.path));
    expect(a).toBe(1);
    expect(b).toBe(1);
    expect(c).toBe(0);
  });
});

describe("effectiveDropped", () => {
  it("a row is dropped if explicitly toggled", () => {
    const rows = flattenSchema(DEEP_SCHEMA);
    const dropped = new Set(["$.id"]);
    const flags = effectiveDropped(rows, (r) => dropped.has(r.path));
    expect(rows.map((r, i) => [r.path, flags[i]])).toEqual([
      ["$.entities", false],
      ["$.entities[*].name", false],
      ["$.entities[*].tags", false],
      ["$.id", true],
    ]);
  });

  it("dropping a node propagates to every descendant, recursively -- the real backend removes the whole subtree via one JSONPath, so a child was never really still there", () => {
    const rows = flattenSchema({
      type: "object",
      properties: {
        a: {
          type: "array",
          items: {
            type: "object",
            properties: { b: { type: "array", items: { type: "object", properties: { c: { type: "string" } } } } },
          },
        },
      },
    });
    const dropped = new Set(["$.a[*].b"]); // drop the middle node, not a leaf
    const flags = effectiveDropped(rows, (r) => dropped.has(r.path));
    expect(rows.map((r, i) => [r.path, flags[i]])).toEqual([
      ["$.a", false], // an ancestor of the dropped node isn't itself dropped
      ["$.a[*].b", true], // explicitly dropped
      ["$.a[*].b[*].c", true], // inherits from its dropped parent
    ]);
  });

  it("composes with countDroppedDescendants so a badge reflects every effectively-dropped descendant, not just explicit toggles", () => {
    const rows = flattenSchema({
      type: "object",
      properties: {
        root: {
          type: "object",
          properties: {
            user: { type: "object", properties: { id: { type: "string" }, name: { type: "string" } } },
          },
        },
      },
    });
    const dropped = new Set(["$.root.user"]); // toggle off "user" itself, not its children
    const flags = effectiveDropped(rows, (r) => dropped.has(r.path));
    const flagByPath = new Map(rows.map((r, i) => [r.path, flags[i]]));
    const counts = countDroppedDescendants(rows, (r) => flagByPath.get(r.path)!);

    // $.root has 3 descendants (user, user.id, user.name) -- all 3 are now
    // effectively dropped, not just the 1 explicitly-toggled "user" row.
    const rootIndex = rows.findIndex((r) => r.path === "$.root");
    expect(counts[rootIndex]).toBe(3);
  });
});

describe("enablePath", () => {
  const USER_SCHEMA = {
    type: "object",
    properties: {
      user: {
        type: "object",
        properties: {
          id: { type: "string" },
          profile: {
            type: "object",
            properties: {
              name: { type: "string" },
              email: { type: "string" },
              address: {
                type: "object",
                properties: { city: { type: "string" }, zip: { type: "string" } },
              },
            },
          },
          roles: { type: "string" },
        },
      },
    },
  };

  it("un-drops a directly (not just effectively) dropped field with a plain removal", () => {
    const rows = flattenSchema(USER_SCHEMA);
    expect(enablePath(rows, ["$.user"], "$.user")).toEqual([]);
  });

  it("returns the drop list unchanged if the target wasn't actually blocked by anything", () => {
    const rows = flattenSchema(USER_SCHEMA);
    expect(enablePath(rows, [], "$.user.profile.name")).toEqual([]);
    expect(enablePath(rows, ["$.user.roles"], "$.user.profile.name")).toEqual(["$.user.roles"]);
  });

  it("un-drops a nested field blocked by a dropped ancestor, preserving every sibling branch as dropped", () => {
    const rows = flattenSchema(USER_SCHEMA);
    const next = enablePath(rows, ["$.user"], "$.user.profile.name");

    // "user" itself is no longer dropped, and neither is "profile" (it's
    // on the path to "name") -- but every sibling branch that was hidden
    // along with "user" stays hidden.
    expect(new Set(next)).toEqual(
      new Set(["$.user.id", "$.user.roles", "$.user.profile.email", "$.user.profile.address"]),
    );
  });

  it("handles a target blocked by a dropped grandparent two levels up the same way", () => {
    const rows = flattenSchema(USER_SCHEMA);
    const next = enablePath(rows, ["$.user"], "$.user.profile.address.city");

    expect(new Set(next)).toEqual(
      new Set([
        "$.user.id",
        "$.user.roles",
        "$.user.profile.name",
        "$.user.profile.email",
        "$.user.profile.address.zip",
      ]),
    );
  });

  it("is defensive against a redundant state where an ancestor and one of its own descendants are both somehow explicitly dropped", () => {
    const rows = flattenSchema(USER_SCHEMA);
    const next = enablePath(rows, ["$.user", "$.user.profile"], "$.user.profile.name");

    expect(new Set(next)).toEqual(
      new Set(["$.user.id", "$.user.roles", "$.user.profile.email", "$.user.profile.address"]),
    );
  });

  it("enabling a branch node (not a leaf) reveals only that node and its ancestors -- not its own descendants", () => {
    // A -> B -> C: dropping A drops B and C in effect. Enabling B should
    // reveal A and B, but C -- never individually requested, just along
    // for the ride under A's original drop -- must stay dropped.
    const rows = flattenSchema({
      type: "object",
      properties: { a: { type: "object", properties: { b: { type: "object", properties: { c: { type: "string" } } } } } },
    });
    const next = enablePath(rows, ["$.a"], "$.a.b");

    expect(next).toEqual(["$.a.b.c"]);
  });

  it("enabling a mid-level branch node re-drops both its siblings and its own children", () => {
    const rows = flattenSchema(USER_SCHEMA);
    const next = enablePath(rows, ["$.user"], "$.user.profile");

    // "user" and "profile" become reachable; "id"/"roles" (profile's
    // siblings) and "name"/"email"/"address" (profile's own children,
    // never asked for) all stay dropped.
    expect(new Set(next)).toEqual(
      new Set([
        "$.user.id",
        "$.user.roles",
        "$.user.profile.name",
        "$.user.profile.email",
        "$.user.profile.address",
      ]),
    );
  });
});
