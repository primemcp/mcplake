import { useMemo, useState } from "react";
import { flattenSchema, matchesFieldQuery } from "../../lib/schema";
import { Card } from "../primitives/Card";
import { SearchInput } from "../primitives/SearchInput";
import type { ToolSchema } from "../../api/types";
import { SchemaFieldRows } from "./SchemaFieldRows";

export type DiscoveredToolsProps = {
  tools: ToolSchema[];
};

/**
 * One search box for the whole section, not one per tool card — a tool
 * with 14 discovered tools would otherwise show 14 separate search boxes
 * (found by actually using the running UI). A tool is visible if its name
 * matches, or any of its input-parameter fields do; a tool matched purely
 * by name shows its full field list (query "" for that card) rather than
 * an oddly-empty card, while a tool matched via a field shows only the
 * matching fields.
 */
export function DiscoveredTools({ tools }: DiscoveredToolsProps) {
  const [query, setQuery] = useState("");

  const visible = useMemo(() => {
    const q = query.trim();
    return tools
      .map((tool) => {
        const fields = flattenSchema(tool.input_schema);
        const fieldHits = fields.filter((f) => matchesFieldQuery(f, q));
        const nameMatches = q === "" || tool.name.toLowerCase().includes(q.toLowerCase());
        return { tool, fieldHits: nameMatches ? fields : fieldHits, matched: nameMatches || fieldHits.length > 0 };
      })
      .filter(({ matched }) => matched);
  }, [tools, query]);

  return (
    <Card className="p-4 flex flex-col gap-2.5">
      <div className="flex flex-wrap items-baseline gap-2.5">
        <div className="text-[13.5px] font-semibold">Discovered tools</div>
        <div className="text-[11.5px] text-muted">
          input parameters — response fields are in Response filters below
        </div>
      </div>

      {tools.length === 0 ? (
        <p className="text-[11.5px] text-subtle">
          No tools discovered yet — the endpoint may still be connecting.
        </p>
      ) : (
        <>
          <SearchInput value={query} onChange={setQuery} placeholder="Search tools or fields" />
          {visible.length === 0 && <p className="text-[11.5px] text-subtle">No tool matches that.</p>}
          <ul className="flex flex-col gap-1.5">
            {visible.map(({ tool }) => (
              <li key={tool.name} className="p-2.5 border border-border rounded-[10px] flex flex-col gap-1.5">
                <div className="text-[12.5px] font-semibold">{tool.name}</div>
                <SchemaFieldRows
                  schema={tool.input_schema}
                  query={
                    query.trim() === "" || tool.name.toLowerCase().includes(query.trim().toLowerCase())
                      ? ""
                      : query
                  }
                />
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  );
}
