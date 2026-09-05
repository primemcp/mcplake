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
  /** How many additional levels of nesting exist below this row — e.g. 14
   * for the root of a 15-deep schema. 0 for a leaf. */
  depthBelow: number;
};

/**
 * Tags each row (in the flat, depth-first order flattenSchema/flattenNode
 * already produce) with whether it has descendants and how deep they go,
 * by scanning forward for a contiguous run of rows at greater indent.
 * O(n^2) worst case, fine for schemas with tens to low hundreds of fields.
 */
export function annotateFields<T extends SchemaField>(rows: T[]): AnnotatedField<T>[] {
  return rows.map((row, i) => {
    let depthBelow = 0;
    for (let j = i + 1; j < rows.length && rows[j].indent > row.indent; j++) {
      depthBelow = Math.max(depthBelow, rows[j].indent - row.indent);
    }
    return { ...row, hasChildren: depthBelow > 0, depthBelow };
  });
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
