import { useState } from "react";
import type { SchemaField } from "../../lib/schema";
import { Tabs } from "../primitives/Tabs";
import { SchemaGraph } from "./SchemaGraph";
import { SchemaTree } from "./SchemaTree";

export type SchemaExplorerProps = {
  rows: SchemaField[];
  selected: string[];
  onToggle: (path: string) => void;
};

const VIEWS = [
  { id: "graph", label: "Graph" },
  { id: "tree", label: "Tree" },
];

/**
 * The schema graph modal's view switcher -- Graph (the node-and-edge
 * view) stays the default since it's the whole reason this modal exists,
 * but a plain nested list reads faster for some schemas (deep, narrow
 * ones especially), so SchemaTree is one tab away rather than a separate
 * entry point. Both views share the same `rows`/`selected`/`onToggle` --
 * toggling a field in one is immediately reflected if you switch to the
 * other.
 */
export function SchemaExplorer({ rows, selected, onToggle }: SchemaExplorerProps) {
  const [view, setView] = useState("graph");

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-2">
      <Tabs tabs={VIEWS} activeId={view} onChange={setView} />
      {view === "graph" ? (
        <SchemaGraph rows={rows} selected={selected} onToggle={onToggle} />
      ) : (
        <SchemaTree rows={rows} selected={selected} onToggle={onToggle} />
      )}
    </div>
  );
}
