import { useMemo, useState } from "react";
import { flattenSchema, matchesFieldQuery } from "../../lib/schema";
import { SearchInput } from "../primitives/SearchInput";
import { SchemaFieldRows } from "./SchemaFieldRows";

export type SchemaFieldPickerProps = {
  schema: unknown;
  selected: string[];
  onToggle: (path: string) => void;
  /** e.g. "tools/list · postgres-ro" — shown next to the field count. */
  meta?: string;
};

/**
 * The mockup's schema browser: a heading + meta line, a search box, the
 * toggleable field list, and a "N fields pass through" summary — used when
 * building/editing a single response filter.
 */
export function SchemaFieldPicker({ schema, selected, onToggle, meta }: SchemaFieldPickerProps) {
  const [query, setQuery] = useState("");
  const rows = useMemo(() => flattenSchema(schema), [schema]);
  const hasHits = useMemo(() => rows.some((r) => matchesFieldQuery(r, query)), [rows, query]);
  const droppedCount = selected.filter((p) => rows.some((r) => r.path === p)).length;

  if (rows.length === 0) {
    return <p className="text-[11px] text-muted px-1">This tool doesn't advertise any fields.</p>;
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
      {hasHits ? (
        <SchemaFieldRows schema={schema} query={query} selected={selected} onToggle={onToggle} />
      ) : (
        <p className="text-[11px] text-muted px-1">No field matches that.</p>
      )}
      {/* Exact mockup phrasing (schemaSummary/schemaSummaryFg): amber once
          anything's dropped, muted while everything still passes through. */}
      <p className={`text-[10.5px] ${droppedCount > 0 ? "text-warn" : "text-muted"}`}>
        {droppedCount === 0
          ? `All ${rows.length} ${rows.length === 1 ? "field" : "fields"} pass through — toggle a field off to strip it`
          : `${droppedCount} of ${rows.length} fields removed from the response`}
      </p>
    </div>
  );
}
