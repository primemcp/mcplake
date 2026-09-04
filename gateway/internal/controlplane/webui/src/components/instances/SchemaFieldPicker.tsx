import { flattenSchema } from "../../lib/schema";

export type SchemaFieldPickerProps = {
  schema: unknown;
  selected: string[];
  onToggle: (path: string) => void;
};

/** The mockup's schema browser: every field the tool's output_schema
 * advertises, indented by nesting depth, with a pill toggle per row — the
 * toggled-on set becomes drop_fields. Matches the source markup's row
 * layout/sizing exactly (see the Admin UI design doc's follow-up on this;
 * this replaces the free-text field input from the first pass). */
export function SchemaFieldPicker({ schema, selected, onToggle }: SchemaFieldPickerProps) {
  const rows = flattenSchema(schema);

  if (rows.length === 0) {
    return (
      <p className="text-[11px] text-subtle px-1">
        This tool's output schema doesn't advertise any fields to pick from.
      </p>
    );
  }

  return (
    <div className="max-h-[200px] overflow-y-auto border border-border rounded-lg bg-surface">
      {rows.map((r) => {
        const on = selected.includes(r.path);
        return (
          <div
            key={r.path}
            className="flex items-center gap-2.5 px-2.5 py-1.5 border-b border-border-soft last:border-b-0"
          >
            <div className="flex-1 min-w-0 flex flex-col gap-px" style={{ paddingLeft: r.indent * 12 }}>
              <div
                className={`text-[11.5px] font-mono truncate ${on ? "text-danger line-through" : "text-body"}`}
              >
                {r.path}
              </div>
              <div className="text-[10px] text-muted">{r.type}</div>
            </div>
            <button
              type="button"
              aria-label={`${on ? "Stop hiding" : "Hide"} ${r.path}`}
              onClick={() => onToggle(r.path)}
              className="shrink-0 border-0 bg-transparent cursor-pointer p-0"
            >
              <span
                className={`flex w-[30px] h-[17px] rounded-full p-0.5 ${on ? "bg-danger justify-end" : "bg-border justify-start"}`}
              >
                <span className="w-[13px] h-[13px] rounded-full bg-white" />
              </span>
            </button>
          </div>
        );
      })}
    </div>
  );
}
