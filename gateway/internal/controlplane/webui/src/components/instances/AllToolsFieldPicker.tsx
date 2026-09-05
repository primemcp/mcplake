import { useMemo, useState } from "react";
import { annotateFields, collapseFields, flattenSchema, matchesFieldQuery, type SchemaField } from "../../lib/schema";
import { Modal } from "../primitives/Modal";
import { SearchInput } from "../primitives/SearchInput";
import type { ToolSchema } from "../../api/types";
import { SchemaGraph } from "./SchemaGraph";

type Row = SchemaField & { tool: string };

// A JSON path never contains a space, so this is a safe, unambiguous
// separator for the composite key even across every tool's own paths.
const SEP = " ";
const key = (tool: string, path: string) => `${tool}${SEP}${path}`;

function flattenAllTools(tools: Record<string, ToolSchema>): Row[] {
  return Object.entries(tools).flatMap(([tool, schema]) =>
    flattenSchema(schema.output_schema).map((f) => ({ ...f, tool })),
  );
}

export type AllToolsFieldPickerProps = {
  tools: Record<string, ToolSchema>;
  /** Reports the field paths dropped so far, plus which tool they belong
   * to (null once the selection is cleared). */
  onChange: (tool: string | null, dropFields: string[]) => void;
  meta?: string;
};

/**
 * The design has no "select a tool" step — one merged, searchable list of
 * every field the endpoint exposes across all its tools (matching the
 * mockup's single `Discovered response fields` browser exactly). The real
 * backend's FilterPolicy still needs exactly one `tool` per filter though
 * (it's a required field, not optional), so rather than an upfront picker,
 * the active tool is inferred from whichever field you actually toggle
 * first — every other tool's fields gray out until you clear the
 * selection. This is the one place this app's data model (many tools per
 * endpoint) genuinely doesn't match the mockup's (one schema per
 * endpoint), so it gets a visible, explained constraint instead of either
 * silently guessing a tool or forcing an up-front dropdown the design
 * never had.
 *
 * Nested fields collapse by default, same as SchemaFieldRows — annotating
 * the whole merged/concatenated row list works unmodified across tool
 * boundaries, since each tool's own rows always start back at indent 0.
 */
export function AllToolsFieldPicker({ tools, onChange, meta }: AllToolsFieldPickerProps) {
  const [query, setQuery] = useState("");
  const [dropped, setDropped] = useState<string[]>([]); // "tool path" keys
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [graphTool, setGraphTool] = useState<string | null>(null);
  const searching = query.trim() !== "";

  const rows = useMemo(() => annotateFields(flattenAllTools(tools)), [tools]);
  const activeTool = dropped.length > 0 ? dropped[0].split(SEP)[0] : null;
  const filtered = useMemo(
    () => (searching ? rows.filter((r) => matchesFieldQuery(r, query)) : rows),
    [rows, query, searching],
  );
  const hits = useMemo(
    () => collapseFields(filtered, (r) => expanded.has(key(r.tool, r.path)), searching),
    [filtered, expanded, searching],
  );
  const droppedCount = dropped.length;

  const toggle = (tool: string, path: string) => {
    const k = key(tool, path);
    const next = dropped.includes(k) ? dropped.filter((x) => x !== k) : [...dropped, k];
    setDropped(next);
    const activeAfter = next.length > 0 ? next[0].split(SEP)[0] : null;
    onChange(
      activeAfter,
      next.filter((x) => x.startsWith(`${activeAfter}${SEP}`)).map((x) => x.split(SEP)[1]),
    );
  };

  const toggleExpanded = (k: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(k)) next.delete(k);
      else next.add(k);
      return next;
    });
  };

  if (rows.length === 0) {
    return <p className="text-[11px] text-muted px-1">No tools have discovered response fields yet.</p>;
  }

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">
          Discovered response fields
        </span>
        <span className="flex-1" />
        {meta && <span className="text-[10px] text-muted font-mono">{meta}</span>}
        <span className="text-[10px] text-muted">
          {rows.length} {rows.length === 1 ? "field" : "fields"}
        </span>
      </div>
      <SearchInput value={query} onChange={setQuery} placeholder="Search fields, e.g. email or PII" />
      {activeTool && (
        <p className="text-[10.5px] text-muted px-1">
          Scoped to <span className="font-mono text-body">{activeTool}</span> — clear the selected
          fields below to filter a different tool instead (one tool per filter).
        </p>
      )}
      {hits.length === 0 ? (
        <p className="text-[11px] text-muted px-1">No field matches that.</p>
      ) : (
        <div className="max-h-[240px] overflow-y-auto border border-border rounded-lg bg-surface">
          {hits.map((r) => {
            const k = key(r.tool, r.path);
            const isDropped = dropped.includes(k);
            const isOpen = expanded.has(k);
            const disabled = activeTool !== null && r.tool !== activeTool;
            const flagged = /PII|secret|internal|PCI|payload|cost|financial/i.test(
              `${r.type} ${r.description ?? ""}`,
            );
            return (
              <div
                key={k}
                className={`flex items-center gap-2.5 px-2.5 py-1.5 border-b border-border-soft last:border-b-0 ${disabled ? "opacity-40" : ""}`}
              >
                {/* 12px per nesting level -- see SchemaFieldRows for why
                    this isn't the design's fixed one-level indent. */}
                <div
                  className="flex-1 min-w-0 flex items-start gap-1.5"
                  style={{ paddingLeft: r.indent * 12 }}
                >
                  {r.hasChildren && !searching ? (
                    <button
                      type="button"
                      disabled={disabled}
                      aria-label={`${isOpen ? "Collapse" : "Expand"} ${r.tool} ${r.path}`}
                      onClick={() => toggleExpanded(k)}
                      className="shrink-0 border-0 bg-transparent cursor-pointer text-[10px] text-muted p-0 w-3.5 leading-[1.6] disabled:cursor-not-allowed"
                    >
                      {isOpen ? "▾" : "▸"}
                    </button>
                  ) : (
                    <span className="shrink-0 w-3.5" />
                  )}
                  {r.hasChildren && (
                    <button
                      type="button"
                      disabled={disabled}
                      aria-label={`View schema graph for ${r.tool}`}
                      title={`View ${r.tool}'s schema as a graph`}
                      onClick={() => setGraphTool(r.tool)}
                      className="shrink-0 border-0 bg-transparent cursor-pointer text-[11px] text-muted p-0 leading-[1.6] disabled:cursor-not-allowed"
                    >
                      👁
                    </button>
                  )}
                  <div className="flex-1 min-w-0 flex flex-col gap-px">
                    <div className="flex items-baseline gap-1.5">
                      <div
                        className={`text-[11.5px] font-mono truncate ${isDropped ? "text-muted line-through" : "text-ink"}`}
                      >
                        <span className="text-muted">{r.tool}:</span> {r.path}
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
                  disabled={disabled}
                  aria-label={`${isDropped ? "Stop hiding" : "Hide"} ${r.tool} ${r.path}`}
                  onClick={() => toggle(r.tool, r.path)}
                  className="shrink-0 border-0 bg-transparent p-0 disabled:cursor-not-allowed cursor-pointer"
                >
                  <span
                    className={`flex w-[30px] h-[17px] rounded-full p-0.5 ${isDropped ? "bg-track-off justify-start" : "bg-success justify-end"}`}
                  >
                    <span className="w-[13px] h-[13px] rounded-full bg-white" />
                  </span>
                </button>
              </div>
            );
          })}
        </div>
      )}
      <p className={`text-[10.5px] ${droppedCount > 0 ? "text-warn" : "text-muted"}`}>
        {droppedCount === 0
          ? `All ${rows.length} ${rows.length === 1 ? "field" : "fields"} pass through — toggle a field off to strip it`
          : `${droppedCount} of ${rows.length} fields removed from the response`}
      </p>
      {/* Scoped to one tool at a time -- unlike the merged list above, a
          graph needs unique node ids, and two different tools can easily
          share a bare path like "$.id". */}
      <Modal open={graphTool !== null} onClose={() => setGraphTool(null)} title="Schema graph" size="wide">
        {graphTool && (
          <SchemaGraph
            rows={rows.filter((r) => r.tool === graphTool)}
            selected={dropped
              .filter((x) => x.startsWith(`${graphTool}${SEP}`))
              .map((x) => x.split(SEP)[1])}
            onToggle={(path) => toggle(graphTool, path)}
          />
        )}
      </Modal>
    </div>
  );
}
