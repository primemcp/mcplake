import { useMemo } from "react";
import { flattenSchema, matchesFieldQuery } from "../../lib/schema";

export type SchemaFieldRowsProps = {
  schema: unknown;
  /** Externally-owned filter text — this component has no search box of
   * its own, so it can be driven by either a per-picker local query
   * (SchemaFieldPicker) or one query shared across many schemas at once
   * (Discovered tools' single search). */
  query: string;
  /** Omit both for a read-only listing — no toggle rendered, nothing
   * clickable. */
  selected?: string[];
  onToggle?: (path: string) => void;
};

/** The scrollable field list itself, with no search box — see
 * SchemaFieldPicker (one schema, its own local search) and Discovered
 * tools (many schemas, one shared search) for the two ways this gets
 * driven. Returns null once filtered to nothing, so a caller listing
 * several schemas can skip an empty one entirely rather than show a
 * dangling "no matches" box per schema. */
export function SchemaFieldRows({ schema, query, selected, onToggle }: SchemaFieldRowsProps) {
  const hits = useMemo(
    () => flattenSchema(schema).filter((r) => matchesFieldQuery(r, query)),
    [schema, query],
  );

  if (hits.length === 0) return null;

  return (
    <div className="max-h-[200px] overflow-y-auto border border-border rounded-lg bg-surface">
      {hits.map((r) => {
        const on = selected?.includes(r.path) ?? false;
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
            {onToggle && (
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
            )}
          </div>
        );
      })}
    </div>
  );
}
