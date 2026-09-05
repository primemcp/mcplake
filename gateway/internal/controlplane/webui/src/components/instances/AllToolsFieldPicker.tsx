import { useMemo, useState } from "react";
import { annotateFields, collapseFields, flattenSchema, matchesFieldQuery, type SchemaField } from "../../lib/schema";
import type { FieldsByTool } from "../../lib/filterGroups";
import { Modal } from "../primitives/Modal";
import { SearchInput } from "../primitives/SearchInput";
import type { ToolSchema } from "../../api/types";
import { SchemaGraph } from "./SchemaGraph";

type Row = SchemaField & { tool: string };

// A JSON path never contains a space, so this is a safe, unambiguous
// separator for the composite key even across every tool's own paths.
const SEP = " ";
const key = (tool: string, path: string) => `${tool}${SEP}${path}`;
const parseKey = (k: string): [tool: string, path: string] => {
  const i = k.indexOf(SEP);
  return [k.slice(0, i), k.slice(i + SEP.length)];
};

function flattenAllTools(tools: Record<string, ToolSchema>): Row[] {
  return Object.entries(tools).flatMap(([tool, schema]) =>
    flattenSchema(schema.output_schema).map((f) => ({ ...f, tool })),
  );
}

function toFieldsByTool(keys: string[]): FieldsByTool {
  const byTool: FieldsByTool = {};
  for (const k of keys) {
    const [tool, path] = parseKey(k);
    (byTool[tool] ??= []).push(path);
  }
  return byTool;
}

export type AllToolsFieldPickerProps = {
  tools: Record<string, ToolSchema>;
  /** Reports every tool with at least one dropped field, keyed by tool
   * name -- called with the full up-to-date map on every toggle. */
  onChange: (fieldsByTool: FieldsByTool) => void;
  /** Prefills the picker when editing an existing (possibly multi-tool)
   * filter group. Only read on mount -- pass a `key` from the parent to
   * force a remount when switching what's being edited. */
  initial?: FieldsByTool;
  meta?: string;
};

/**
 * The design has no "select a tool" step — one merged, searchable list of
 * every field the endpoint exposes across all its tools (matching the
 * mockup's single `Discovered response fields` browser exactly). The real
 * backend's FilterPolicy still needs exactly one `tool` per record though
 * (it's a required field, not a list) — so a "filter across N tools" here
 * is a frontend-only grouping of N real per-tool records sharing a name
 * prefix (see lib/filterGroups). This picker itself doesn't need to know
 * about that: it just reports every tool with at least one dropped field,
 * and the caller (ResponseFilterGroup) decides how to persist that as one
 * or several real FilterPolicy records.
 *
 * Nested fields collapse by default -- annotating the whole
 * merged/concatenated row list works unmodified across tool boundaries,
 * since each tool's own rows always start back at indent 0.
 */
export function AllToolsFieldPicker({ tools, onChange, initial, meta }: AllToolsFieldPickerProps) {
  const [query, setQuery] = useState("");
  const [dropped, setDropped] = useState<string[]>(() =>
    Object.entries(initial ?? {}).flatMap(([tool, paths]) => paths.map((p) => key(tool, p))),
  );
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [graphTool, setGraphTool] = useState<string | null>(null);
  const searching = query.trim() !== "";

  const rows = useMemo(() => annotateFields(flattenAllTools(tools)), [tools]);
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
    onChange(toFieldsByTool(next));
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
      {hits.length === 0 ? (
        <p className="text-[11px] text-muted px-1">No field matches that.</p>
      ) : (
        <div className="max-h-[240px] overflow-y-auto border border-border rounded-lg bg-surface">
          {hits.map((r) => {
            const k = key(r.tool, r.path);
            const isDropped = dropped.includes(k);
            const isOpen = expanded.has(k);
            const flagged = /PII|secret|internal|PCI|payload|cost|financial/i.test(
              `${r.type} ${r.description ?? ""}`,
            );
            return (
              <div
                key={k}
                className="flex items-center gap-2.5 px-2.5 py-1.5 border-b border-border-soft last:border-b-0"
              >
                {/* 12px per nesting level, not the design's fixed
                    one-level indent -- real schemas can nest arbitrarily
                    deep, so indent needs to scale with actual depth. */}
                <div
                  className="flex-1 min-w-0 flex items-start gap-1.5"
                  style={{ paddingLeft: r.indent * 12 }}
                >
                  {r.hasChildren && !searching ? (
                    <button
                      type="button"
                      aria-label={`${isOpen ? "Collapse" : "Expand"} ${r.tool} ${r.path}`}
                      onClick={() => toggleExpanded(k)}
                      className="shrink-0 border-0 bg-transparent cursor-pointer text-[10px] text-muted p-0 w-3.5 leading-[1.6]"
                    >
                      {isOpen ? "▾" : "▸"}
                    </button>
                  ) : (
                    <span className="shrink-0 w-3.5" />
                  )}
                  {r.hasChildren && (
                    <button
                      type="button"
                      aria-label={`View schema graph for ${r.tool}`}
                      title={`View ${r.tool}'s schema as a graph`}
                      onClick={() => setGraphTool(r.tool)}
                      className="shrink-0 border-0 bg-transparent cursor-pointer text-[11px] text-muted p-0 leading-[1.6]"
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
                        <span className="shrink-0 text-[9.5px] text-muted font-mono">[{r.descendantCount}]</span>
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
                  aria-label={`${isDropped ? "Stop hiding" : "Hide"} ${r.tool} ${r.path}`}
                  onClick={() => toggle(r.tool, r.path)}
                  className="shrink-0 border-0 bg-transparent p-0 cursor-pointer"
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
