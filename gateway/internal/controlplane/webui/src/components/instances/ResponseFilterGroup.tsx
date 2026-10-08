import { useMemo, useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { Input } from "../primitives/Input";
import { SearchInput } from "../primitives/SearchInput";
import type { FilterPolicy, MCPRegistration } from "../../api/types";
import { AllToolsFieldPicker } from "./AllToolsFieldPicker";
import { GROUP_SEP, groupFilters, planGroupSave, type FieldsByTool, type FilterGroup } from "../../lib/filterGroups";

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

/** One line per dropped field, tool-prefixed the same way the picker's own
 * rows are -- reads the same for a plain single-tool filter and a group
 * spanning several, no special-casing needed. */
function groupSummary(g: FilterGroup): string {
  const dropped = g.members.flatMap((m) => m.drop_fields.map((p) => `${m.tool}: ${p}`));
  if (dropped.length === 0) {
    return `${[...new Set(g.members.map((m) => m.tool))].join(", ")} · full response`;
  }
  const shown = dropped.slice(0, 3).join(" · ");
  const rest = dropped.length > 3 ? ` +${dropped.length - 3}` : "";
  return `${shown}${rest} removed`;
}

function groupToFieldsByTool(g: FilterGroup): FieldsByTool {
  const byTool: FieldsByTool = {};
  for (const m of g.members) byTool[m.tool] = m.drop_fields;
  return byTool;
}

function EditGroupForm({
  group,
  endpoint,
  onSave,
  onCancel,
  onDeleteGroup,
}: {
  group: FilterGroup;
  endpoint: MCPRegistration;
  onSave: (fieldsByTool: FieldsByTool) => Promise<void>;
  onCancel: () => void;
  onDeleteGroup: () => Promise<void>;
}) {
  const [fieldsByTool, setFieldsByTool] = useState<FieldsByTool>(() => groupToFieldsByTool(group));
  const [saving, setSaving] = useState(false);

  return (
    <div className="p-3 border border-accent rounded-lg bg-form-soft flex flex-col gap-2">
      <div className="text-[11.5px] font-semibold">Edit response filter</div>
      <AllToolsFieldPicker
        tools={endpoint.tools ?? {}}
        initial={groupToFieldsByTool(group)}
        onChange={setFieldsByTool}
        meta={`tools/list · ${endpoint.name}`}
      />
      <div className="flex flex-wrap items-center gap-1.5">
        <Button
          onClick={async () => {
            setSaving(true);
            await onSave(fieldsByTool);
          }}
          disabled={saving}
        >
          Save filter
        </Button>
        <Button variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
        <span className="flex-1" />
        <Button variant="danger" onClick={onDeleteGroup}>
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
  const [editingId, setEditingId] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [fieldsByTool, setFieldsByTool] = useState<FieldsByTool>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [listQuery, setListQuery] = useState("");

  const tools = Object.keys(endpoint.tools ?? {});
  const scoped = filters.filter((f) => f.mcp === endpoint.name);
  // A "filter" in this UI can be several real FilterPolicy records (one per
  // tool -- the backend has no other way to span tools), grouped purely by
  // a `<name>::<tool>` prefix on each record's own `name`. See
  // lib/filterGroups for why, and why a plain legacy name with no `::`
  // still works unmodified as its own single-member group.
  const groups = useMemo(() => groupFilters(scoped), [scoped]);
  const editingGroup = groups.find((g) => g.id === editingId) ?? null;
  // No filters yet: skip the empty-state message and go straight to an
  // open creation form instead of making the operator click "+" first.
  const formOpen = tools.length > 0 && (adding || (groups.length === 0 && !loading && !error));

  // Mirrors the mockup's single "Search filters or fields" box above the
  // list — distinct from AllToolsFieldPicker's own per-form field search,
  // which narrows one filter's candidate schema fields while building it.
  const visible = useMemo(() => {
    const q = listQuery.trim().toLowerCase();
    if (q === "") return groups;
    return groups.filter(
      (g) =>
        g.id.toLowerCase().includes(q) ||
        g.members.some(
          (m) => m.tool.toLowerCase().includes(q) || m.drop_fields.some((field) => field.toLowerCase().includes(q)),
        ),
    );
  }, [groups, listQuery]);
  const shown = expanded ? visible : visible.slice(0, VISIBLE_LIMIT);
  const hiddenCount = visible.length - shown.length;

  // Diffs the desired per-tool selection against what's actually persisted
  // for this group, then fires only the real API calls the change needs:
  // existing members keep their name (PUT), new tools mint a
  // `<group>::<tool>` name (POST), tools dropped from the selection
  // entirely get removed (DELETE). No rollback on partial failure -- this
  // is an admin tool operating on independent backend records, not one
  // atomic transaction.
  const runPlan = async (groupId: string, existing: FilterPolicy[], desired: FieldsByTool) => {
    const plan = planGroupSave(groupId, existing, desired);
    await Promise.all([
      ...plan.toCreate.map((c) => onCreate(c.name, c.tool, c.dropFields)),
      ...plan.toUpdate.map((u) => onUpdate(u.name, u.tool, u.dropFields)),
      ...plan.toDelete.map((n) => onDelete(n)),
    ]);
  };

  // A name containing the group separator would, once minted as
  // `<name>::<tool>`, groupIdOf() back down to whatever's before its own
  // *first* "::" -- silently merging this filter's real records into an
  // unrelated existing group with that same prefix (and making them
  // editable/deletable together). Rejected at input time rather than
  // sanitized, since silently stripping/replacing it would let two
  // different typed names collide into the same group without any
  // indication why.
  const nameHasSeparator = name.includes(GROUP_SEP);

  const submit = async () => {
    if (Object.keys(fieldsByTool).length === 0 || nameHasSeparator) return;
    setSubmitError(null);
    try {
      await runPlan(name.trim(), [], fieldsByTool);
      setAdding(false);
      setName("");
      setFieldsByTool({});
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
        groups.length > 0 && (
          <>
            <SearchInput value={listQuery} onChange={setListQuery} placeholder="Search filters or fields" />
            {visible.length === 0 && <p className="text-[11.5px] text-subtle">No filter matches that.</p>}
            <ul className="flex flex-col gap-1.5">
              {shown.map((g) =>
                editingGroup?.id === g.id ? (
                  <li key={g.id}>
                    <EditGroupForm
                      group={g}
                      endpoint={endpoint}
                      onCancel={() => setEditingId(null)}
                      onSave={async (desired) => {
                        await runPlan(g.id, g.members, desired);
                        setEditingId(null);
                      }}
                      onDeleteGroup={async () => {
                        await Promise.all(g.members.map((m) => onDelete(m.name)));
                        setEditingId(null);
                      }}
                    />
                  </li>
                ) : (
                  <li
                    key={g.id}
                    className="flex items-center gap-3 p-2.5 border border-border rounded-[10px]"
                  >
                    <div className="flex-1 min-w-0 flex flex-col gap-0.5">
                      <div className="text-[12.5px] font-medium truncate">{g.id}</div>
                      <div className="text-[10.5px] font-mono text-subtle truncate">{groupSummary(g)}</div>
                    </div>
                    {/* Always "not used yet" until #80 (Users & access) exists to
                        compute a real per-filter usage count from access grants —
                        not a fabricated number in the meantime. */}
                    <span className="shrink-0 text-[10.5px] text-muted whitespace-nowrap">
                      not used yet
                    </span>
                    <button
                      type="button"
                      onClick={() => setEditingId(g.id)}
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
          <label className="flex flex-col gap-1">
            <span className="text-[11px] font-semibold text-subtle">
              Filter name <span className="text-danger">*</span>
            </span>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="filter name" required />
            {nameHasSeparator && (
              <span className="text-[10.5px] text-danger">
                Filter names can't contain "{GROUP_SEP}" (reserved for internal grouping).
              </span>
            )}
          </label>
          <AllToolsFieldPicker
            tools={endpoint.tools ?? {}}
            onChange={setFieldsByTool}
            meta={`tools/list · ${endpoint.name}`}
          />
          {submitError && <p className="text-[10.5px] text-danger">{submitError}</p>}
          <div className="flex gap-1.5">
            <Button
              onClick={submit}
              disabled={name.trim() === "" || nameHasSeparator || Object.keys(fieldsByTool).length === 0}
            >
              Create filter
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setAdding(false);
                setFieldsByTool({});
              }}
            >
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
