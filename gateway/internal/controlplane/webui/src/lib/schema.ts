export type SchemaField = { path: string; type: string; indent: number };

type JSONSchema = {
  type?: string;
  properties?: Record<string, JSONSchema>;
  items?: JSONSchema;
};

/**
 * Flattens a tool's output_schema (JSON Schema draft-07, as returned by
 * GET /admin/mcps) into the field rows the response-filter picker toggles
 * — matching what the mockup's schema browser assumed the gateway would
 * expose. Handles the common case (object/properties, array/items,
 * primitives); schemas using anyOf/oneOf are left as a single "any" leaf
 * rather than expanded, since none of the real MCP servers tested against
 * this app use them for output schemas.
 */
export function flattenSchema(schema: unknown, basePath = "$", indent = 0): SchemaField[] {
  if (schema == null || typeof schema !== "object") return [];
  const s = schema as JSONSchema;

  if (s.type === "object" && s.properties) {
    return Object.entries(s.properties).flatMap(([key, child]) => {
      const path = `${basePath}.${key}`;
      const childType = child?.type ?? "any";
      if (childType === "object" && child.properties) {
        return [{ path, type: childType, indent }, ...flattenSchema(child, path, indent + 1)];
      }
      if (childType === "array") {
        const itemType = child.items?.type ?? "any";
        return [{ path, type: `array<${itemType}>`, indent }];
      }
      return [{ path, type: childType, indent }];
    });
  }
  return [];
}

/** Case-insensitive match against a field's path or type — shared by every
 * schema-field search box (SchemaFieldPicker's own, and the single search
 * that spans every tool card in Discovered tools) so "narrows by path or
 * type" means the same thing everywhere. */
export function matchesFieldQuery(field: SchemaField, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (q === "") return true;
  return field.path.toLowerCase().includes(q) || field.type.toLowerCase().includes(q);
}
