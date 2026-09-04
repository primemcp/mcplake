import { flattenSchema } from "../../lib/schema";

export type ToolFieldListProps = {
  schema: unknown;
};

/** Read-only field listing for a tool's card in "Discovered tools" —
 * same flattenSchema() used by SchemaFieldPicker, just rendered as a
 * plain bulleted list instead of a toggle picker, since there's nothing
 * to select here (see #79's discussion: output_schema fields are
 * already covered by the Response filters picker below; this shows
 * input_schema, the tool's parameters). */
export function ToolFieldList({ schema }: ToolFieldListProps) {
  const rows = flattenSchema(schema);

  if (rows.length === 0) {
    return <p className="text-[10.5px] text-muted">No parameters.</p>;
  }

  return (
    <ul className="flex flex-col gap-0.5">
      {rows.map((r) => (
        <li
          key={r.path}
          className="flex items-baseline gap-1.5 text-[10.5px] font-mono"
          style={{ paddingLeft: r.indent * 12 }}
        >
          <span className="text-muted">*</span>
          <span className="text-subtle">{r.path}</span>
          <span className="text-muted">{r.type}</span>
        </li>
      ))}
    </ul>
  );
}
