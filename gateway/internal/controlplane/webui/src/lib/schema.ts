export type SchemaField = { path: string; type: string; indent: number; description?: string };

type JSONSchema = {
  type?: string;
  description?: string;
  properties?: Record<string, JSONSchema>;
  items?: JSONSchema;
};

/**
 * Flattens a tool's output_schema (JSON Schema draft-07, as returned by
 * GET /admin/mcps) into the field rows the response-filter picker toggles.
 * Handles the common case (object/properties, array/items, primitives);
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
    return Object.entries(s.properties).flatMap(([key, child]) => {
      const path = `${basePath}.${key}`;
      const childType = child?.type ?? "any";
      if (childType === "object" && child.properties) {
        return [
          { path, type: childType, indent, description: child.description },
          ...flattenSchema(child, path, indent + 1),
        ];
      }
      if (childType === "array") {
        const itemType = child.items?.type ?? "any";
        return [{ path, type: `array<${itemType}>`, indent, description: child.description }];
      }
      return [{ path, type: childType, indent, description: child.description }];
    });
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
