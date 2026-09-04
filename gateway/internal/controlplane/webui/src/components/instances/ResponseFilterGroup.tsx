import { useMemo, useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { Input } from "../primitives/Input";
import { SearchInput } from "../primitives/SearchInput";
import type { FilterPolicy, MCPRegistration } from "../../api/types";
import { SchemaFieldPicker } from "./SchemaFieldPicker";

export type ResponseFilterGroupProps = {
  endpoint: MCPRegistration;
  filters: FilterPolicy[];
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  onCreate: (name: string, tool: string, dropFields: string[]) => Promise<void>;
  onUpdate: (name: string, tool: string, dropFields: string[]) => Promise<void>;
  onDelete: (name: string) => Promise<void>;
};

// How many filter rows show before "Show N more filters" collapses the rest.
const VISIBLE_LIMIT = 3;

// Matches the mockup's own row summary exactly: first 3 dropped fields
// joined by " · ", "+N" for the rest, "full response" when nothing's
// dropped. Our model needs the tool name prefixed (the mockup assumes one
// schema per endpoint; a real endpoint can expose several tools).
function filterSummary(f: FilterPolicy): string {
  if (f.drop_fields.length === 0) return `${f.tool} · full response`;
  const shown = f.drop_fields.slice(0, 3).join(" · ");
  const rest = f.drop_fields.length > 3 ? ` +${f.drop_fields.length - 3}` : "";
  return `${f.tool} · ${shown}${rest} removed`;
}

function EditFilterForm({
  filter,
  endpoint,
  onSave,
  onCancel,
  onDelete,
}: {
  filter: FilterPolicy;
  endpoint: MCPRegistration;
  onSave: (dropFields: string[]) => Promise<void>;
  onCancel: () => void;
  onDelete: () => Promise<void>;
}) {
  const [fields, setFields] = useState<string[]>(filter.drop_fields);
  const [saving, setSaving] = useState(false);
  const outputSchema = endpoint.tools?.[filter.tool]?.output_schema;

  return (
    <div className="p-3 border border-accent rounded-lg bg-form-soft flex flex-col gap-2">
      <div className="text-[11.5px] font-semibold">Edit response filter</div>
      <SchemaFieldPicker
        schema={outputSchema}
        selected={fields}
        onToggle={(path) =>
          setFields((prev) => (prev.includes(path) ? prev.filter((p) => p !== path) : [...prev, path]))
        }
        meta={`tools/list · ${endpoint.name}`}
      />
      <div className="flex flex-wrap items-center gap-1.5">
        <Button
          onClick={async () => {
            setSaving(true);
            await onSave(fields);
          }}
          disabled={saving}
        >
          Save filter
        </Button>
        <Button variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
        <span className="flex-1" />
        <Button variant="danger" onClick={onDelete}>
          Delete
        </Button>
      </div>
    </div>
  );
}

export function ResponseFilterGroup({
  endpoint,
  filters,
  loading,
  error,
  onRetry,
  onCreate,
  onUpdate,
  onDelete,
}: ResponseFilterGroupProps) {
  const [adding, setAdding] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [tool, setTool] = useState("");
  const [fields, setFields] = useState<string[]>([]);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [listQuery, setListQuery] = useState("");

  const tools = Object.keys(endpoint.tools ?? {});
  const scoped = filters.filter((f) => f.mcp === endpoint.name);
  const editingFilter = scoped.find((f) => f.name === editingName) ?? null;
  // No filters yet: skip the empty-state message and go straight to an
  // open creation form instead of making the operator click "+" first.
  const formOpen = tools.length > 0 && (adding || (scoped.length === 0 && !loading && !error));

  // Mirrors the mockup's single "Search filters or fields" box above the
  // list — distinct from SchemaFieldPicker's own per-form field search,
  // which narrows one filter's candidate schema fields while building it.
  const visible = useMemo(() => {
    const q = listQuery.trim().toLowerCase();
    if (q === "") return scoped;
    return scoped.filter(
      (f) =>
        f.name.toLowerCase().includes(q) ||
        f.tool.toLowerCase().includes(q) ||
        f.drop_fields.some((field) => field.toLowerCase().includes(q)),
    );
  }, [scoped, listQuery]);
  const shown = expanded ? visible : visible.slice(0, VISIBLE_LIMIT);
  const hiddenCount = visible.length - shown.length;

  const submit = async () => {
    setSubmitError(null);
    try {
      await onCreate(name.trim(), tool.trim(), fields);
      setAdding(false);
      setName("");
      setTool("");
      setFields([]);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Failed to create filter.");
    }
  };

  return (
    <Card className="p-4 flex flex-col gap-2.5">
      <div className="flex flex-wrap items-baseline gap-2.5">
        <div className="text-[13.5px] font-semibold">Response filters</div>
        <div className="text-[11.5px] text-muted">built from this endpoint's discovered response schema</div>
      </div>

      {error ? (
        <ErrorNotice onRetry={onRetry} />
      ) : loading ? (
        <p className="text-[11.5px] text-subtle">Loading…</p>
      ) : (
        scoped.length > 0 && (
          <>
            <SearchInput value={listQuery} onChange={setListQuery} placeholder="Search filters or fields" />
            {visible.length === 0 && <p className="text-[11.5px] text-subtle">No filter matches that.</p>}
            <ul className="flex flex-col gap-1.5">
              {shown.map((f) =>
                editingFilter?.name === f.name ? (
                  <li key={f.name}>
                    <EditFilterForm
                      filter={f}
                      endpoint={endpoint}
                      onCancel={() => setEditingName(null)}
                      onSave={async (dropFields) => {
                        await onUpdate(f.name, f.tool, dropFields);
                        setEditingName(null);
                      }}
                      onDelete={async () => {
                        await onDelete(f.name);
                        setEditingName(null);
                      }}
                    />
                  </li>
                ) : (
                  <li
                    key={f.name}
                    className="flex items-center gap-3 p-2.5 border border-border rounded-[10px]"
                  >
                    <div className="flex-1 min-w-0 flex flex-col gap-0.5">
                      <div className="text-[12.5px] font-medium truncate">{f.name}</div>
                      <div className="text-[10.5px] font-mono text-subtle truncate">
                        {filterSummary(f)}
                      </div>
                    </div>
                    {/* Always "not used yet" until #80 (Users & access) exists to
                        compute a real per-filter usage count from access grants —
                        not a fabricated number in the meantime. */}
                    <span className="shrink-0 text-[10.5px] text-muted whitespace-nowrap">
                      not used yet
                    </span>
                    <button
                      type="button"
                      onClick={() => setEditingName(f.name)}
                      className="shrink-0 px-2.5 py-1 border border-border rounded-md bg-surface cursor-pointer text-[11px] font-medium text-body hover:border-accent hover:text-accent"
                    >
                      Edit
                    </button>
                  </li>
                ),
              )}
            </ul>
            {hiddenCount > 0 && (
              <button
                type="button"
                onClick={() => setExpanded(true)}
                className="self-start border-0 bg-transparent cursor-pointer text-[11.5px] font-medium text-accent p-0"
              >
                Show {hiddenCount} more {hiddenCount === 1 ? "filter" : "filters"}
              </button>
            )}
            {expanded && visible.length > VISIBLE_LIMIT && (
              <button
                type="button"
                onClick={() => setExpanded(false)}
                className="self-start border-0 bg-transparent cursor-pointer text-[11.5px] font-medium text-accent p-0"
              >
                Hide {visible.length - VISIBLE_LIMIT} {visible.length - VISIBLE_LIMIT === 1 ? "filter" : "filters"}
              </button>
            )}
          </>
        )
      )}

      {formOpen ? (
        <div className="p-3 border border-accent rounded-lg bg-form-soft flex flex-col gap-2">
          <div className="text-xs font-semibold">New response filter</div>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="filter name" />
          <select
            value={tool}
            onChange={(e) => {
              setTool(e.target.value);
              setFields([]);
            }}
            className="px-2.5 py-2 border border-border rounded-lg text-[12.5px] font-mono"
          >
            <option value="">select a tool…</option>
            {tools.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
          {tool && (
            <SchemaFieldPicker
              schema={endpoint.tools?.[tool]?.output_schema}
              selected={fields}
              onToggle={(path) =>
                setFields((prev) =>
                  prev.includes(path) ? prev.filter((p) => p !== path) : [...prev, path],
                )
              }
              meta={`tools/list · ${endpoint.name}`}
            />
          )}
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
          className="flex items-center gap-2.5 p-2.5 border border-dashed border-line rounded-[10px] bg-surface cursor-pointer text-left disabled:opacity-50 disabled:cursor-not-allowed"
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
