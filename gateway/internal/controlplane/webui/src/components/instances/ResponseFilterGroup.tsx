import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { Input } from "../primitives/Input";
import type { FilterPolicy, MCPRegistration } from "../../api/types";

export type ResponseFilterGroupProps = {
  endpoint: MCPRegistration;
  filters: FilterPolicy[];
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  onCreate: (name: string, tool: string, dropFields: string[]) => Promise<void>;
  onDelete: (name: string) => Promise<void>;
};

function parseFields(raw: string): string[] {
  return raw
    .split(",")
    .map((f) => f.trim())
    .filter((f) => f !== "");
}

/**
 * Simplified relative to the mockup: rather than browsing the endpoint's
 * discovered output_schema field-by-field with toggles, this pass takes
 * drop_fields as a plain comma-separated field-path list. The schema is
 * already available per tool (endpoint.tools[tool].output_schema) for a
 * follow-up to build the full picker against.
 */
export function ResponseFilterGroup({
  endpoint,
  filters,
  loading,
  error,
  onRetry,
  onCreate,
  onDelete,
}: ResponseFilterGroupProps) {
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [tool, setTool] = useState("");
  const [fields, setFields] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);

  const tools = Object.keys(endpoint.tools ?? {});
  const scoped = filters.filter((f) => f.mcp === endpoint.name);

  const submit = async () => {
    setSubmitError(null);
    try {
      await onCreate(name.trim(), tool.trim(), parseFields(fields));
      setAdding(false);
      setName("");
      setTool("");
      setFields("");
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Failed to create filter.");
    }
  };

  return (
    <Card className="p-4 flex flex-col gap-2.5">
      <div className="text-[13.5px] font-semibold">Response filters</div>

      {error ? (
        <ErrorNotice onRetry={onRetry} />
      ) : loading ? (
        <p className="text-[11.5px] text-subtle">Loading…</p>
      ) : scoped.length === 0 ? (
        <p className="text-[11.5px] text-subtle">
          No response filter on this endpoint yet — every field of its response reaches the
          agent.
        </p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {scoped.map((f) => (
            <li
              key={f.name}
              className="flex items-center gap-3 p-2.5 border border-border rounded-[10px]"
            >
              <div className="flex-1 min-w-0 flex flex-col gap-0.5">
                <div className="text-[12.5px] font-medium truncate">{f.name}</div>
                <div className="text-[10.5px] font-mono text-subtle truncate">
                  {f.tool}: {f.drop_fields.join(", ")}
                </div>
              </div>
              <Button variant="danger" onClick={() => onDelete(f.name)}>
                Delete
              </Button>
            </li>
          ))}
        </ul>
      )}

      {adding ? (
        <div className="p-3 border border-accent rounded-lg bg-accent-soft flex flex-col gap-2">
          <div className="text-xs font-semibold">New response filter</div>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="filter name" />
          <select
            value={tool}
            onChange={(e) => setTool(e.target.value)}
            className="px-2.5 py-2 border border-border rounded-lg text-[12.5px] font-mono"
          >
            <option value="">select a tool…</option>
            {tools.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
          <Input
            value={fields}
            onChange={(e) => setFields(e.target.value)}
            placeholder="fields to drop, comma-separated (e.g. $.salary, $.ssn)"
            mono
          />
          {submitError && <p className="text-[10.5px] text-danger">{submitError}</p>}
          <div className="flex gap-1.5">
            <Button onClick={submit} disabled={name.trim() === "" || tool.trim() === ""}>
              Create filter
            </Button>
            <Button variant="secondary" onClick={() => setAdding(false)}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setAdding(true)}
          disabled={tools.length === 0}
          className="flex items-center gap-2.5 p-2.5 border border-dashed border-border rounded-[10px] bg-surface cursor-pointer text-left disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <span className="w-4 h-4 rounded border border-dashed border-muted grid place-items-center text-[11px] text-muted">
            +
          </span>
          <span className="text-[12px] font-semibold">
            {tools.length === 0 ? "No tools discovered yet" : "Add response filter"}
          </span>
        </button>
      )}
    </Card>
  );
}
