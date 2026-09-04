import { useMemo } from "react";
import { flattenSchema, matchesFieldQuery } from "../../lib/schema";

export type SchemaFieldRowsProps = {
  schema: unknown;
  /** Externally-owned filter text — this component has no search box of
   * its own. */
  query: string;
  selected: string[];
  onToggle: (path: string) => void;
};

// Matches the mockup's own `/PII|secret|internal|PCI|payload|cost|financial/`
// flag regex — there it's tested against a hardcoded demo type string
// ("string · PII"); here it's tested against a real field's type +
// description, since our schemas come from real MCPs, not fixture data.
const SENSITIVE_HINT = /PII|secret|internal|PCI|payload|cost|financial/i;

/** The scrollable, toggleable field list itself, with no search box or
 * heading — see SchemaFieldPicker for those.
 *
 * Toggle color is a positive framing, not a warning one: green = this
 * field passes through untouched, gray = it's been toggled off (dropped).
 * That's backwards from what you'd guess ("red means I'm hiding it") but
 * matches the mockup's own trackBg logic exactly (`off ? '#dfe3ea' :
 * C.ok`) — getting this inverted was the "colors are wrong" bug.
 */
export function SchemaFieldRows({ schema, query, selected, onToggle }: SchemaFieldRowsProps) {
  const hits = useMemo(
    () => flattenSchema(schema).filter((r) => matchesFieldQuery(r, query)),
    [schema, query],
  );

  if (hits.length === 0) return null;

  return (
    <div className="max-h-[200px] overflow-y-auto border border-border rounded-lg bg-surface">
      {hits.map((r) => {
        const dropped = selected.includes(r.path);
        const flagged = SENSITIVE_HINT.test(`${r.type} ${r.description ?? ""}`);
        return (
          <div
            key={r.path}
            className="flex items-center gap-2.5 px-2.5 py-1.5 border-b border-border-soft last:border-b-0"
          >
            <div
              className="flex-1 min-w-0 flex flex-col gap-px"
              style={{ paddingLeft: r.indent > 0 ? 14 : 0 }}
            >
              <div
                className={`text-[11.5px] font-mono truncate ${dropped ? "text-muted line-through" : "text-ink"}`}
              >
                {r.path}
              </div>
              <div className={`text-[10px] ${flagged ? "text-warn" : "text-muted"}`}>
                {r.type}
                {r.description ? ` · ${r.description}` : ""}
              </div>
            </div>
            <button
              type="button"
              aria-label={`${dropped ? "Stop hiding" : "Hide"} ${r.path}`}
              onClick={() => onToggle(r.path)}
              className="shrink-0 border-0 bg-transparent cursor-pointer p-0"
            >
              <span
                className={`flex w-[30px] h-[17px] rounded-full p-0.5 ${dropped ? "bg-track-off justify-start" : "bg-success justify-end"}`}
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
