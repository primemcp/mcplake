import { useEffect, useMemo, useRef, useState } from "react";
import {
  annotateFields,
  countDroppedDescendants,
  effectiveDropped,
  matchesFieldQuery,
  type SchemaField,
} from "../../lib/schema";
import { SearchInput } from "../primitives/SearchInput";

export type SchemaTreeProps = {
  /** The full, unfiltered row list -- same invariant as SchemaGraph: this
   * view exists specifically to see the whole nested structure at once,
   * not to browse top-level-first like AllToolsFieldPicker's own list. */
  rows: SchemaField[];
  selected: string[];
  onToggle: (path: string) => void;
};

/**
 * The graph's "just show me the JSON shape" alternative -- a plain
 * indented tree of every field, for schemas where reading a nested list
 * is faster than tracing edges. A checkbox rather than a pill switch:
 * "select this field" reads more naturally as a checkbox in a tree, and
 * it's the standard control for a multi-select list (matches AccessTab's
 * own per-tool checkboxes).
 *
 * Search never hides a row here, only highlights matches -- a hit deep in
 * the tree stays visible with its surrounding structure intact instead of
 * collapsing down to just the hits, which is exactly the "see the whole
 * shape" reason this view exists in the first place.
 */
export function SchemaTree({ rows, selected, onToggle }: SchemaTreeProps) {
  const [query, setQuery] = useState("");
  const searching = query.trim() !== "";
  const listRef = useRef<HTMLDivElement>(null);

  // A highlighted match scrolled off-screen (very likely in a schema deep
  // enough to need this view in the first place) is indistinguishable
  // from no match at all -- searching only *filters* if you can already
  // see the result, so it has to also bring the first hit into view.
  useEffect(() => {
    if (!searching) return;
    listRef.current?.querySelector<HTMLElement>("[data-match]")?.scrollIntoView({ block: "nearest" });
  }, [query, searching]);

  const annotated = useMemo(() => annotateFields(rows), [rows]);
  const selectedSet = useMemo(() => new Set(selected), [selected]);
  const effectiveByPath = useMemo(() => {
    const effective = effectiveDropped(annotated, (r) => selectedSet.has(r.path));
    const m = new Map<string, boolean>();
    annotated.forEach((r, i) => m.set(r.path, effective[i]));
    return m;
  }, [annotated, selectedSet]);
  const droppedBelowByPath = useMemo(() => {
    const counts = countDroppedDescendants(annotated, (r) => effectiveByPath.get(r.path) ?? false);
    const m = new Map<string, number>();
    annotated.forEach((r, i) => m.set(r.path, counts[i]));
    return m;
  }, [annotated, effectiveByPath]);

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-2">
      <SearchInput value={query} onChange={setQuery} placeholder="Search fields, e.g. email or PII" />
      <div ref={listRef} className="flex-1 min-h-0 overflow-y-auto border border-border rounded-lg bg-surface">
        {annotated.map((r) => {
          const isDropped = effectiveByPath.get(r.path) ?? false;
          const below = droppedBelowByPath.get(r.path) ?? 0;
          const isMatch = searching && matchesFieldQuery(r, query);
          return (
            <label
              key={r.path}
              data-match={isMatch ? "true" : undefined}
              className={`flex items-center gap-2.5 px-2.5 py-1.5 border-b border-border-soft last:border-b-0 cursor-pointer ${
                isMatch ? "bg-warn-soft" : ""
              }`}
            >
              <input type="checkbox" checked={!isDropped} onChange={() => onToggle(r.path)} aria-label={r.path} />
              <div className="flex-1 min-w-0 flex flex-col gap-px" style={{ paddingLeft: r.indent * 14 }}>
                <div className="flex items-baseline gap-1.5">
                  <span
                    className={`text-[11.5px] font-mono truncate ${isDropped ? "text-muted line-through" : "text-ink"}`}
                  >
                    {r.path}
                  </span>
                  {below > 0 && <span className="text-[9.5px] text-danger">-{below}</span>}
                </div>
                <div className="text-[10px] text-muted">
                  {r.type}
                  {r.description ? ` · ${r.description}` : ""}
                </div>
              </div>
            </label>
          );
        })}
      </div>
    </div>
  );
}
