import { useMemo, useState } from "react";
import { flattenSchema } from "../../lib/schema";
import { SearchInput } from "../primitives/SearchInput";

export type SchemaFieldPickerProps = {
  schema: unknown;
  /** Omit both selected/onToggle for a read-only listing (e.g. Discovered
   * tools' input parameters) — no toggle is rendered and nothing is
   * clickable, but search/scroll behave identically either way. */
  selected?: string[];
  onToggle?: (path: string) => void;
};

/** The mockup's schema browser: a search box over every field the schema
 * advertises, indented by nesting depth, in a scrollable list — matches
 * the source markup's field-search + max-height-260px pattern (used here
 * at both call sites: picking response-filter fields with a toggle, and
 * read-only browsing of a tool's input parameters without one). */
export function SchemaFieldPicker({ schema, selected, onToggle }: SchemaFieldPickerProps) {
  const [query, setQuery] = useState("");
  const rows = useMemo(() => flattenSchema(schema), [schema]);
  const hits = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === "") return rows;
    return rows.filter(
      (r) => r.path.toLowerCase().includes(q) || r.type.toLowerCase().includes(q),
    );
  }, [rows, query]);

  if (rows.length === 0) {
    return <p className="text-[11px] text-muted px-1">This tool doesn't advertise any fields.</p>;
  }

  return (
    <div className="flex flex-col gap-1.5">
      <SearchInput value={query} onChange={setQuery} placeholder="Search fields, e.g. email or PII" />
      {hits.length === 0 ? (
        <p className="text-[11px] text-muted px-1">No field matches that.</p>
      ) : (
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
      )}
    </div>
  );
}
