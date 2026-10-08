export type SchemaField = { path: string; type: string; indent: number; description?: string };

type JSONSchema = {
  type?: string;
  description?: string;
  properties?: Record<string, JSONSchema>;
  items?: JSONSchema;
};

// Labels a leaf array type recursively, e.g. an array of arrays of strings
// becomes "array<array<string>>" rather than stopping at "array<array>".
function describeType(node: JSONSchema | undefined): string {
  const type = node?.type ?? "any";
  if (type === "array") return `array<${describeType(node?.items)}>`;
  return type;
}

// Flattens one property's schema into its own row, plus (recursively) its
// children when there's something meaningful to drill into:
//   - a nested object: its own properties, indented one level further
//   - an array of objects: the *item* schema's properties, addressed with
//     the JSONPath wildcard `[*]` (e.g. "$.entities[*].name") -- this is
//     real, backend-supported syntax (see filter/filter.go's Strip, built
//     on github.com/theory/jsonpath: "$.items[*].secret" is its own
//     doc-comment example), not a frontend-only convenience
//   - an array of primitives/arrays: a leaf, described recursively
//     ("array<array<string>>") since there's no object to expand into
function flattenNode(node: JSONSchema | undefined, path: string, indent: number): SchemaField[] {
  const type = node?.type ?? "any";
  const description = node?.description;

  if (type === "object" && node?.properties) {
    return [
      { path, type, indent, description },
      ...Object.entries(node.properties).flatMap(([key, child]) =>
        flattenNode(child, `${path}.${key}`, indent + 1),
      ),
    ];
  }

  if (type === "array") {
    const items = node?.items;
    if (items?.type === "object" && items.properties) {
      const itemPath = `${path}[*]`;
      return [
        { path, type: "array<object>", indent, description },
        ...Object.entries(items.properties).flatMap(([key, child]) =>
          flattenNode(child, `${itemPath}.${key}`, indent + 1),
        ),
      ];
    }
    return [{ path, type: describeType(node), indent, description }];
  }

  return [{ path, type, indent, description }];
}

/**
 * Flattens a tool's output_schema (JSON Schema draft-07, as returned by
 * GET /admin/mcps) into the field rows the response-filter picker toggles.
 * Recurses through nested objects and arrays-of-objects (see flattenNode);
 * schemas using anyOf/oneOf are left as a single "any" leaf rather than
 * expanded, since none of the real MCP servers tested against this app use
 * them for output schemas.
 *
 * description is carried through as-is (standard JSON Schema, not a
 * heuristic) — an MCP author who documents a field as e.g. "PII" or
 * "secret" in its schema surfaces that verbatim next to the type; nothing
 * here infers sensitivity on its own.
 */
export function flattenSchema(schema: unknown, basePath = "$", indent = 0): SchemaField[] {
  if (schema == null || typeof schema !== "object") return [];
  const s = schema as JSONSchema;

  if (s.type === "object" && s.properties) {
    return Object.entries(s.properties).flatMap(([key, child]) =>
      flattenNode(child, `${basePath}.${key}`, indent),
    );
  }
  return [];
}

/** Case-insensitive match against a field's path, type, or description —
 * shared by every schema-field search box so "narrows by field" means the
 * same thing everywhere it's used. */
export function matchesFieldQuery(field: SchemaField, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (q === "") return true;
  return (
    field.path.toLowerCase().includes(q) ||
    field.type.toLowerCase().includes(q) ||
    (field.description?.toLowerCase().includes(q) ?? false)
  );
}

export type AnnotatedField<T extends SchemaField = SchemaField> = T & {
  hasChildren: boolean;
  /** Total number of descendant rows below this one, recursively — not
   * just direct children. A row with two leaf children is 2, same as a
   * row with one child that itself has one child (both are "2 fields
   * live under here"), which is the number that actually matches what
   * you see once you expand it. */
  descendantCount: number;
};

/**
 * Tags each row (in the flat, depth-first order flattenSchema/flattenNode
 * already produce) with whether it has descendants and how many, by
 * scanning forward for a contiguous run of rows at greater indent. O(n^2)
 * worst case, fine for schemas with tens to low hundreds of fields.
 */
export function annotateFields<T extends SchemaField>(rows: T[]): AnnotatedField<T>[] {
  return rows.map((row, i) => {
    let descendantCount = 0;
    for (let j = i + 1; j < rows.length && rows[j].indent > row.indent; j++) {
      descendantCount++;
    }
    return { ...row, hasChildren: descendantCount > 0, descendantCount };
  });
}

/**
 * For every row, counts how many of its descendants (per the same
 * contiguous-indent scan as annotateFields) are currently dropped —
 * lets a row that's collapsed in the browse view (nested fields are only
 * ever toggled through the schema graph now, not an inline expand) still
 * show at a glance that something underneath it has been filtered,
 * without opening the graph to check.
 */
export function countDroppedDescendants<T extends SchemaField>(
  rows: T[],
  isDropped: (row: T) => boolean,
): number[] {
  return rows.map((row, i) => {
    let count = 0;
    for (let j = i + 1; j < rows.length && rows[j].indent > row.indent; j++) {
      if (isDropped(rows[j])) count++;
    }
    return count;
  });
}

/**
 * Whether each row is dropped *in effect* -- explicitly toggled off, or a
 * descendant of a row that is (explicitly or, recursively, in effect).
 * Dropping a field removes its whole subtree via one JSONPath (real
 * backend semantics, see filter.Strip()), so a child was never really
 * "still there" just because nobody toggled it individually -- browsing/
 * the graph should show that, not just the one row someone actually
 * clicked.
 *
 * Walked in the same depth-first order as annotateFields/countDroppedDescendants,
 * tracking the nearest dropped ancestor's indent per level: a row inherits
 * "dropped" from whichever tracked ancestor is directly above it, and
 * itself becomes the tracked ancestor for its own indent going forward.
 */
export function effectiveDropped<T extends SchemaField>(
  rows: T[],
  isExplicitlyDropped: (row: T) => boolean,
): boolean[] {
  const droppedAtIndent = new Map<number, boolean>();
  return rows.map((row) => {
    const parentDropped = droppedAtIndent.get(row.indent - 1) ?? false;
    const dropped = parentDropped || isExplicitlyDropped(row);
    droppedAtIndent.set(row.indent, dropped);
    return dropped;
  });
}

/**
 * "Enabling" a field that's only dropped *in effect* (blocked by a dropped
 * ancestor, per effectiveDropped above) can't just remove it from
 * `dropped` -- it was never explicitly there. Un-dropping the whole
 * ancestor instead would resurrect every sibling subtree that was hidden
 * along with it, which is very likely not what was wanted (nothing else
 * was touched). This computes the minimal edit that makes exactly
 * `target` itself reachable -- and nothing more: every sibling branch
 * along the path from the blocking ancestor down to `target` stays
 * dropped, and so does everything *below* `target` (its own descendants
 * were never asked for either -- enabling a branch node reveals that one
 * node, not the subtree hanging off it).
 *
 * When `target` is itself explicitly dropped (the common, direct case --
 * toggling the exact thing you dropped back on), this is a plain removal
 * instead, restoring its whole subtree: that's a single deliberate
 * on/off action, not a "reach one specific nested field" one, so there's
 * nothing to preserve.
 */
export function enablePath<T extends SchemaField>(
  rows: T[],
  dropped: string[],
  target: string,
): string[] {
  if (dropped.includes(target)) {
    return dropped.filter((p) => p !== target);
  }

  const targetIndex = rows.findIndex((r) => r.path === target);
  if (targetIndex === -1) return dropped;

  // Ancestor chain (shallowest first), via the same "nearest preceding
  // row at each shallower indent" technique buildGraph uses to find a
  // row's parent.
  const lastAtIndent = new Map<number, T>();
  for (let i = 0; i < targetIndex; i++) lastAtIndent.set(rows[i].indent, rows[i]);
  const chain: T[] = [];
  for (let d = 0; d < rows[targetIndex].indent; d++) {
    const row = lastAtIndent.get(d);
    if (row) chain.push(row);
  }

  const droppedSet = new Set(dropped);
  const blockers = chain.filter((r) => droppedSet.has(r.path));
  if (blockers.length === 0) return dropped; // wasn't actually blocked

  const next = new Set(dropped);
  for (const b of blockers) next.delete(b.path);

  const directChildren = (parent: T): T[] => {
    const parentIndex = rows.indexOf(parent);
    const children: T[] = [];
    for (let i = parentIndex + 1; i < rows.length && rows[i].indent > parent.indent; i++) {
      if (rows[i].indent === parent.indent + 1) children.push(rows[i]);
    }
    return children;
  };

  const shallowestBlockerIndex = chain.findIndex((r) => droppedSet.has(r.path));
  const pathFromBlocker = [...chain.slice(shallowestBlockerIndex), rows[targetIndex]];
  for (let i = 0; i < pathFromBlocker.length - 1; i++) {
    const parent = pathFromBlocker[i];
    const onPath = pathFromBlocker[i + 1];
    for (const child of directChildren(parent)) {
      if (child.path !== onPath.path) next.add(child.path);
    }
  }

  // `target` itself becoming reachable doesn't mean its own descendants
  // should too -- nobody asked for those, they were just along for the
  // ride under the original ancestor drop. Re-drop target's own direct
  // children unconditionally (there's no "onPath" child to preserve here,
  // unlike the loop above) so enabling a *branch* node lets you reach
  // exactly that node and no further.
  for (const child of directChildren(rows[targetIndex])) {
    next.add(child.path);
  }

  return [...next];
}

/**
 * Filters an annotated, depth-first-ordered row list down to what should
 * actually render given which rows are expanded — collapsed by default
 * (isExpanded returning false hides that row's entire subtree, nested
 * collapses included) unless `searching` is true, in which case every row
 * is shown regardless of expand state: search always searches everything,
 * collapse only affects browsing.
 */
export function collapseFields<T extends { indent: number; hasChildren: boolean }>(
  rows: T[],
  isExpanded: (row: T) => boolean,
  searching: boolean,
): T[] {
  if (searching) return rows;

  const result: T[] = [];
  let hideBelowIndent: number | null = null;
  for (const row of rows) {
    if (hideBelowIndent !== null) {
      if (row.indent > hideBelowIndent) continue;
      hideBelowIndent = null;
    }
    result.push(row);
    if (row.hasChildren && !isExpanded(row)) {
      hideBelowIndent = row.indent;
    }
  }
  return result;
}

export type GraphNode = { id: string; label: string; type: string; description?: string };
export type GraphEdge = { id: string; source: string; target: string };

// Strips "$." and every "[*]" wildcard marker, keeping just the final
// segment -- a compact node label ("name" rather than
// "$.entities[*].name") since the graph shows the full path via the edges
// themselves.
function lastSegment(path: string): string {
  const cleaned = path.replace(/\[\*\]/g, "");
  const segments = cleaned.split(".");
  return segments[segments.length - 1] || path;
}

/**
 * Builds a node-and-edge graph from a flat, depth-first-ordered row list —
 * works for either one schema's full row list or several independent
 * top-level trees concatenated together (as long as each tree's own rows
 * are contiguous and start back at indent 0, which is exactly what
 * flattenSchema/flattenAllTools always produce).
 *
 * A row's parent is the nearest *preceding* row one indent level shallower
 * -- tracked with one slot per indent level, overwritten as rows are
 * walked in order, which is enough because deeper rows always immediately
 * follow their own parent in this ordering before any shallower sibling
 * can appear.
 */
export function buildGraph(rows: SchemaField[]): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const nodes: GraphNode[] = rows.map((r) => ({
    id: r.path,
    label: lastSegment(r.path),
    type: r.type,
    description: r.description,
  }));

  const edges: GraphEdge[] = [];
  const lastAtIndent = new Map<number, string>();
  for (const r of rows) {
    if (r.indent > 0) {
      const parentId = lastAtIndent.get(r.indent - 1);
      if (parentId) edges.push({ id: `${parentId}->${r.path}`, source: parentId, target: r.path });
    }
    lastAtIndent.set(r.indent, r.path);
  }

  return { nodes, edges };
}
