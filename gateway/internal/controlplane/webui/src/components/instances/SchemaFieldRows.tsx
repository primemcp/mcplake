import { useMemo, useState } from "react";
import { annotateFields, collapseFields, flattenSchema, matchesFieldQuery } from "../../lib/schema";

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
 *
 * Nested fields collapse by default (found deeply-nested real schemas —
 * see fake-deep2's 15-level test — made a fully-flattened list unwieldy).
 * A search in progress bypasses collapse entirely: it always matches
 * against every field regardless of expand state, only the browse view
 * (empty query) hides collapsed subtrees.
 */
export function SchemaFieldRows({ schema, query, selected, onToggle }: SchemaFieldRowsProps) {
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const searching = query.trim() !== "";

  const annotated = useMemo(() => annotateFields(flattenSchema(schema)), [schema]);
  const filtered = useMemo(
    () => (searching ? annotated.filter((r) => matchesFieldQuery(r, query)) : annotated),
    [annotated, query, searching],
  );
  const hits = useMemo(
    () => collapseFields(filtered, (r) => expanded.has(r.path), searching),
    [filtered, expanded, searching],
  );

  const toggleExpanded = (path: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  if (hits.length === 0) return null;

  return (
    <div className="max-h-[200px] overflow-y-auto border border-border rounded-lg bg-surface">
      {hits.map((r) => {
        const dropped = selected.includes(r.path);
        const flagged = SENSITIVE_HINT.test(`${r.type} ${r.description ?? ""}`);
        const isOpen = expanded.has(r.path);
        return (
          <div
            key={r.path}
            className="flex items-center gap-2.5 px-2.5 py-1.5 border-b border-border-soft last:border-b-0"
          >
            {/* 12px per nesting level, not the design's fixed one-level
                14px — real schemas (unlike the mockup's flat demo data)
                can nest arbitrarily deep (object-in-array-in-object...),
                so indent needs to scale with actual depth. */}
            <div
              className="flex-1 min-w-0 flex items-start gap-1.5"
              style={{ paddingLeft: r.indent * 12 }}
            >
              {r.hasChildren && !searching ? (
                <button
                  type="button"
                  aria-label={`${isOpen ? "Collapse" : "Expand"} ${r.path}`}
                  onClick={() => toggleExpanded(r.path)}
                  className="shrink-0 border-0 bg-transparent cursor-pointer text-[10px] text-muted p-0 w-3.5 leading-[1.6]"
                >
                  {isOpen ? "▾" : "▸"}
                </button>
              ) : (
                <span className="shrink-0 w-3.5" />
              )}
              <div className="flex-1 min-w-0 flex flex-col gap-px">
                <div className="flex items-baseline gap-1.5">
                  <div
                    className={`text-[11.5px] font-mono truncate ${dropped ? "text-muted line-through" : "text-ink"}`}
                  >
                    {r.path}
                  </div>
                  {r.hasChildren && (
                    <span className="shrink-0 text-[9.5px] text-muted font-mono">[{r.depthBelow}]</span>
                  )}
                </div>
                <div className={`text-[10px] ${flagged ? "text-warn" : "text-muted"}`}>
                  {r.type}
                  {r.description ? ` · ${r.description}` : ""}
                </div>
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
