import { useMemo, useState } from "react";
import { flattenSchema, matchesFieldQuery } from "../../lib/schema";
import { SearchInput } from "../primitives/SearchInput";
import { SchemaFieldRows } from "./SchemaFieldRows";

export type SchemaFieldPickerProps = {
  schema: unknown;
  /** Omit both selected/onToggle for a read-only listing — no toggle is
   * rendered and nothing is clickable, but search/scroll behave
   * identically either way. */
  selected?: string[];
  onToggle?: (path: string) => void;
};

/** One schema, its own search box: used when building/editing a single
 * response filter, where the field list belongs to exactly one selected
 * tool. For browsing many schemas' fields under one shared search instead
 * (Discovered tools), use SchemaFieldRows directly with an externally-owned
 * query. */
export function SchemaFieldPicker({ schema, selected, onToggle }: SchemaFieldPickerProps) {
  const [query, setQuery] = useState("");
  const rows = useMemo(() => flattenSchema(schema), [schema]);
  const hasHits = useMemo(() => rows.some((r) => matchesFieldQuery(r, query)), [rows, query]);

  if (rows.length === 0) {
    return <p className="text-[11px] text-muted px-1">This tool doesn't advertise any fields.</p>;
  }

  return (
    <div className="flex flex-col gap-1.5">
      <SearchInput value={query} onChange={setQuery} placeholder="Search fields, e.g. email or PII" />
      {hasHits ? (
        <SchemaFieldRows schema={schema} query={query} selected={selected} onToggle={onToggle} />
      ) : (
        <p className="text-[11px] text-muted px-1">No field matches that.</p>
      )}
    </div>
  );
}
