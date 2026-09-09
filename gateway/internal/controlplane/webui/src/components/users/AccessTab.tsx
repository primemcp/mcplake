import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { Input } from "../primitives/Input";
import { SearchInput } from "../primitives/SearchInput";
import { statusFromString } from "../primitives/StatusDot";
import { Toggle } from "../primitives/Toggle";
import { AllToolsFieldPicker } from "../instances/AllToolsFieldPicker";
import {
  GROUP_SEP,
  groupFilters,
  groupIdOf,
  planGroupSave,
  stripGroupPrefix,
  type FieldsByTool,
  type FilterGroup,
} from "../../lib/filterGroups";
import type { ClaimRule, FilterPolicy, FilterPolicyRequest, Grant, MCPRegistration, ToolSchema } from "../../api/types";

export type AccessTabProps = {
  endpoints: MCPRegistration[];
  grants: Grant[];
  onGrantsChange: (grants: Grant[]) => void;
  /** Every real FilterPolicy in the system -- step 2 shows only the ones
   * grouped under `userName` (see composeUsers/ADR-0008's convention). */
  allFilters: FilterPolicy[];
  onGoInstances: () => void;
  /** null while drafting a brand new, not-yet-saved user -- there's no
   * real AccessPolicy name yet for a filter to be grouped under, so step
   * 2 shows a hint instead of the filter editor (see below). */
  userName: string | null;
  /** The user's current (possibly still-unsaved) match rules -- every
   * filter created or edited here carries this, kept in sync the same
   * way TokenMatchTab's own doc comment describes. */
  userMatch: ClaimRule[];
  /** Response filters save immediately as they're created/edited/deleted
   * (matching ResponseFilterGroup's own behavior on the MCP connections
   * screen) rather than waiting for the panel's Save button -- unlike
   * Step 1's grants, a FilterPolicy is a fully independent backend
   * record with no atomic-transaction relationship to the AccessPolicy
   * at all, so there's nothing for a shared "draft" to buy here. */
  onCreateFilter: (req: FilterPolicyRequest) => Promise<void>;
  onUpdateFilter: (name: string, req: FilterPolicyRequest) => Promise<void>;
  onDeleteFilter: (name: string) => Promise<void>;
};

type ConnFilter = "all" | "granted" | "connected" | "pending";

// How many filter rows show per endpoint before "Show N more filters"
// collapses the rest -- mirrors ResponseFilterGroup's own VISIBLE_LIMIT.
const VISIBLE_LIMIT = 3;

function toolsForGrant(endpoint: MCPRegistration, grant: Grant): Record<string, ToolSchema> {
  const all = endpoint.tools ?? {};
  if (grant.tools.includes("*")) return all;
  return Object.fromEntries(Object.entries(all).filter(([name]) => grant.tools.includes(name)));
}

function isConnected(endpoint: MCPRegistration): boolean {
  return statusFromString(endpoint.status) === "active";
}

function matchesQuery(endpoint: MCPRegistration, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (q === "") return true;
  const haystack = `${endpoint.name} ${endpoint.connect.command ?? ""} ${endpoint.connect.url ?? ""}`.toLowerCase();
  return haystack.includes(q);
}

function groupToFieldsByTool(g: FilterGroup): FieldsByTool {
  const byTool: FieldsByTool = {};
  for (const m of g.members) byTool[m.tool] = m.drop_fields;
  return byTool;
}

/** One line per dropped field, tool-prefixed, plus a total-count badge --
 * matches ResponseFilterGroup's own groupSummary on the MCP connections
 * screen (with the badge added, per the real mockup's own filter rows). */
function groupSummary(g: FilterGroup): string {
  const dropped = g.members.flatMap((m) => m.drop_fields.map((p) => `${m.tool}: ${p}`));
  if (dropped.length === 0) {
    return `${[...new Set(g.members.map((m) => m.tool))].join(", ")} · full response`;
  }
  const shown = dropped.slice(0, 3).join(" · ");
  const rest = dropped.length > 3 ? ` +${dropped.length - 3}` : "";
  return `${shown}${rest} [${dropped.length}]`;
}

/**
 * Step 1 (endpoint picker) and step 2 (per-endpoint response filters) of
 * the mockup's Access tab. Step 1 is purely controlled (owns no save
 * state of its own, since the panel's sticky footer commits grants/match
 * together; see UserDetail) -- step 2 is not: see the `onCreateFilter`
 * doc comment above for why filters save immediately instead.
 *
 * Fetched the real mockup source via DesignSync to match this
 * structurally rather than the flatter, guessed-at layout an earlier
 * pass had. Two disclosed differences from that same source:
 *
 * - No endpoint tags: the mockup's tag pills/tag-filter row are its own
 *   fictional data (the real `MCPRegistration` has no `Tags` field, per
 *   the frontend design doc's "Data model reconciliation" section) --
 *   showing them would mean fabricating data, so they're left out.
 * - Per-tool narrowing (the "All tools" toggle + checklist below a
 *   granted endpoint) is kept even though the mockup's own Step 1 doesn't
 *   show one -- granting is always-every-tool there. The real backend's
 *   `Grant.tools` genuinely supports a subset, and this was already built
 *   and tested before the mockup was re-checked; removing a real,
 *   working capability just to match a simplified reference would be a
 *   regression, not a fidelity fix.
 *
 * Step 2 reproduces the mockup's actual pattern: one endpoint can carry
 * several independently-named filters (e.g. "Mask customer PII", "Strip
 * query plans"), each its own row with a summary + Edit, sitting on a
 * card backed by the endpoint it belongs to -- not the single
 * always-merged picker an earlier pass built. This reuses
 * lib/filterGroups.ts's grouping (the exact convention
 * ResponseFilterGroup already uses on the MCP connections screen) one
 * level under the `<user>::` prefix: a filter named
 * `<user>::<label>::<tool>` groups under `<user>` for composeUsers, and
 * -- once that prefix is stripped -- under `<label>` here, so one user
 * can have several distinctly-named filters per endpoint exactly like an
 * operator can on MCP connections.
 *
 * The mockup's per-group "N of M selected" implies picking from a shared
 * pool of already-existing filters -- the real backend has no such pool
 * (a FilterPolicy belongs to no one but itself, and ADR-0002's
 * ClaimMatcher is AND-only, so one record can't correctly match two
 * users with different conditions; see ADR-0010). This shows a plain
 * count instead, and "start from an existing filter" (inside the Edit/
 * Add form) clones another filter's `drop_fields` as a one-time starting
 * point rather than a live, shared selection.
 */
export function AccessTab({
  endpoints,
  grants,
  onGrantsChange,
  allFilters,
  onGoInstances,
  userName,
  userMatch,
  onCreateFilter,
  onUpdateFilter,
  onDeleteFilter,
}: AccessTabProps) {
  const [epQuery, setEpQuery] = useState("");
  const [connFilter, setConnFilter] = useState<ConnFilter>("all");
  const [filQuery, setFilQuery] = useState("");
  // `${mcp}::${label}` of the one filter group currently expanded for
  // editing -- only one at a time, matching ResponseFilterGroup's own
  // editingId. `${mcp}::__new__` means that endpoint's "add filter" form.
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [expandedMcps, setExpandedMcps] = useState<Set<string>>(new Set());
  const [newName, setNewName] = useState("");
  const [newFields, setNewFields] = useState<FieldsByTool>({});
  const [submitError, setSubmitError] = useState<string | null>(null);

  const grantedNames = new Set(grants.map((g) => g.mcp));
  const grantedEndpoints = endpoints.filter((e) => grantedNames.has(e.name));

  const setGrantTools = (mcp: string, tools: string[]) => {
    onGrantsChange(grants.map((g) => (g.mcp === mcp ? { ...g, tools } : g)));
  };

  const toggleEndpoint = (endpoint: MCPRegistration) => {
    if (!isConnected(endpoint)) return;
    const mcp = endpoint.name;
    if (grantedNames.has(mcp)) {
      onGrantsChange(grants.filter((g) => g.mcp !== mcp));
    } else {
      onGrantsChange([...grants, { mcp, tools: ["*"] }]);
    }
  };

  const toggleAllTools = (endpoint: MCPRegistration, useAll: boolean) => {
    setGrantTools(endpoint.name, useAll ? ["*"] : Object.keys(endpoint.tools ?? {}));
  };

  const toggleTool = (endpoint: MCPRegistration, grant: Grant, tool: string) => {
    const current = grant.tools.includes("*") ? Object.keys(endpoint.tools ?? {}) : grant.tools;
    const next = current.includes(tool) ? current.filter((t) => t !== tool) : [...current, tool];
    setGrantTools(endpoint.name, next);
  };

  const filteredEndpoints = endpoints.filter((e) => {
    if (!matchesQuery(e, epQuery)) return false;
    if (connFilter === "granted") return grantedNames.has(e.name);
    if (connFilter === "connected") return isConnected(e);
    if (connFilter === "pending") return !isConnected(e);
    return true;
  });
  const connectedSelectable = filteredEndpoints.filter(isConnected);
  const allSelected = connectedSelectable.length > 0 && connectedSelectable.every((e) => grantedNames.has(e.name));

  const toggleAllEndpoints = () => {
    if (allSelected) {
      onGrantsChange(grants.filter((g) => !connectedSelectable.some((e) => e.name === g.mcp)));
    } else {
      const toAdd = connectedSelectable
        .filter((e) => !grantedNames.has(e.name))
        .map((e): Grant => ({ mcp: e.name, tools: ["*"] }));
      onGrantsChange([...grants, ...toAdd]);
    }
  };

  const connTabs: { id: ConnFilter; label: string; count: number }[] = [
    { id: "all", label: "All", count: endpoints.length },
    { id: "granted", label: "Granted", count: grantedNames.size },
    { id: "connected", label: "Connected", count: endpoints.filter(isConnected).length },
    { id: "pending", label: "Not connected", count: endpoints.filter((e) => !isConnected(e)).length },
  ];

  // This user's own filters for one endpoint, grouped by label (the
  // `<user>::` prefix already stripped so groupFilters' own groupIdOf
  // clusters by the *next* segment instead).
  const groupsFor = (mcp: string): FilterGroup[] => {
    if (!userName) return [];
    const mine = allFilters
      .filter((f) => f.mcp === mcp && groupIdOf(f.name) === userName)
      .map((f) => ({ ...f, name: stripGroupPrefix(f.name, userName) }));
    return groupFilters(mine);
  };

  const closeEditors = () => {
    setEditingKey(null);
    setNewName("");
    setNewFields({});
    setSubmitError(null);
  };

  const runPlan = async (mcp: string, groupId: string, existing: FilterPolicy[], desired: FieldsByTool) => {
    if (!userName) return;
    const plan = planGroupSave(groupId, existing, desired);
    const real = (n: string) => `${userName}${GROUP_SEP}${n}`;
    await Promise.all([
      ...plan.toCreate.map((c) =>
        onCreateFilter({ name: real(c.name), match: userMatch, mcp, tool: c.tool, drop_fields: c.dropFields }),
      ),
      ...plan.toUpdate.map((u) =>
        onUpdateFilter(real(u.name), { name: real(u.name), match: userMatch, mcp, tool: u.tool, drop_fields: u.dropFields }),
      ),
      ...plan.toDelete.map((n) => onDeleteFilter(real(n))),
    ]);
  };

  const deleteGroup = async (group: FilterGroup) => {
    if (!userName) return;
    await Promise.all(group.members.map((m) => onDeleteFilter(`${userName}${GROUP_SEP}${m.name}`)));
    closeEditors();
  };

  const nameHasSeparator = newName.includes(GROUP_SEP);

  const submitNew = async (mcp: string) => {
    if (Object.keys(newFields).length === 0 || newName.trim() === "" || nameHasSeparator) return;
    setSubmitError(null);
    try {
      await runPlan(mcp, newName.trim(), [], newFields);
      closeEditors();
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "Failed to create filter.");
    }
  };

  if (endpoints.length === 0) {
    return <p className="text-[11px] text-muted px-1">No MCP connections registered yet.</p>;
  }

  return (
    <div className="h-full min-h-0 grid grid-cols-2 gap-px bg-border-soft border border-border-soft rounded-lg overflow-hidden">
      {/* Step 1's own header/filters/search stay put -- only the endpoint
          list below them scrolls, matching the mockup's own nested
          overflow-y-auto around just its `epCards` list, not the whole
          column. Step 2 (the other grid column) scrolls as one unit
          instead, also per the mockup. */}
      <section className="bg-surface p-3.5 flex flex-col gap-2 min-h-0 overflow-hidden border-t-[3px] border-accent">
        <div className="flex items-center gap-2 shrink-0">
          <span className="w-[18px] h-[18px] rounded-full bg-well grid place-items-center text-[10.5px] font-bold font-mono text-body">
            1
          </span>
          <span className="text-[12px] font-semibold">MCP endpoints</span>
          <span className="flex-1" />
          <button
            type="button"
            onClick={toggleAllEndpoints}
            disabled={connectedSelectable.length === 0}
            className="border-0 bg-transparent p-0 cursor-pointer text-[11px] font-medium text-accent disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {allSelected ? "Clear all" : "Select all"}
          </button>
        </div>

        <div className="flex gap-1.5 flex-wrap shrink-0">
          {connTabs.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => setConnFilter(t.id)}
              className={`px-2.5 py-1 rounded-full text-[11px] font-medium border cursor-pointer ${
                connFilter === t.id ? "bg-ink text-white border-ink" : "bg-surface text-body border-border"
              }`}
            >
              {t.label} {t.count}
            </button>
          ))}
        </div>

        <div className="shrink-0">
          <SearchInput value={epQuery} onChange={setEpQuery} placeholder="Search endpoints, URLs" />
        </div>

        {filteredEndpoints.length === 0 && (
          <p className="text-[11px] text-muted px-1 shrink-0">No endpoint matches that.</p>
        )}

        <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-1.5">
          {filteredEndpoints.map((endpoint) => {
            const grant = grants.find((g) => g.mcp === endpoint.name);
            const granted = grant !== undefined;
            const connected = isConnected(endpoint);
            const allTools = Object.keys(endpoint.tools ?? {});
            const isAllTools = grant?.tools.includes("*") ?? true;
            const checkedTools = new Set(grant ? (isAllTools ? allTools : grant.tools) : []);
            const filterCount = allFilters.filter((f) => f.mcp === endpoint.name).length;

            return (
              <Card key={endpoint.name} className="shrink-0 overflow-hidden">
                <div className={`flex items-center gap-1 p-[10px_12px] ${granted ? "bg-select-soft" : "bg-surface"}`}>
                  <button
                    type="button"
                    role="switch"
                    aria-checked={granted}
                    aria-label={`Grant ${endpoint.name}`}
                    disabled={!connected}
                    onClick={() => toggleEndpoint(endpoint)}
                    className={`flex-1 min-w-0 text-left flex items-start gap-2.5 border-0 bg-transparent p-0 ${
                      connected ? "cursor-pointer" : "cursor-not-allowed opacity-70"
                    }`}
                  >
                    <span
                      className={`w-4 h-4 rounded border grid place-items-center text-[10px] font-bold text-white mt-0.5 shrink-0 ${
                        granted ? "bg-accent border-accent" : "bg-white border-line"
                      }`}
                    >
                      {granted ? "✓" : ""}
                    </span>
                    <div className="flex-1 min-w-0 flex flex-col gap-0.5">
                      <div className="flex items-center gap-[7px] min-w-0">
                        <span
                          className={`w-[7px] h-[7px] rounded-full shrink-0 ${connected ? "bg-success" : "bg-warn animate-pulse"}`}
                        />
                        <span className="text-[12.5px] font-semibold truncate">{endpoint.name}</span>
                      </div>
                      <div className="text-[10.5px] font-mono text-subtle truncate">
                        {endpoint.connect.command || endpoint.connect.url || "—"}
                      </div>
                      <div className="text-[10.5px] text-muted">
                        {filterCount} {filterCount === 1 ? "filter" : "filters"}
                        {granted ? " · access granted" : ""}
                        {!connected ? " · not connected" : ""}
                      </div>
                    </div>
                  </button>
                  <button
                    type="button"
                    onClick={onGoInstances}
                    title="Open in MCP connections"
                    aria-label={`Open ${endpoint.name} in MCP connections`}
                    className="shrink-0 self-start border border-border rounded-md bg-surface px-1.5 py-0.5 cursor-pointer text-[11px] text-muted hover:border-accent hover:text-accent"
                  >
                    ↗
                  </button>
                </div>

                {granted && allTools.length > 0 && (
                  <div className="px-3 pb-2.5 pt-2 pl-[38px] border-t border-border-soft flex flex-col gap-1.5">
                    <Toggle
                      checked={isAllTools}
                      onChange={(v) => toggleAllTools(endpoint, v)}
                      label="All tools"
                      aria-label={`All tools for ${endpoint.name}`}
                    />
                    <div className="flex flex-wrap gap-x-3 gap-y-1">
                      {allTools.map((tool) => (
                        <label key={tool} className="flex items-center gap-1.5 text-[11.5px] font-mono">
                          <input
                            type="checkbox"
                            checked={checkedTools.has(tool)}
                            onChange={() => toggleTool(endpoint, grant, tool)}
                          />
                          {tool}
                        </label>
                      ))}
                    </div>
                  </div>
                )}
              </Card>
            );
          })}
        </div>
      </section>

      <section className="bg-surface p-3.5 flex flex-col gap-2 min-h-0 overflow-y-auto border-t-[3px] border-line">
        <div className="flex items-center gap-2 shrink-0">
          <span className="w-[18px] h-[18px] rounded-full bg-well grid place-items-center text-[10.5px] font-bold font-mono text-body">
            2
          </span>
          <span className="text-[12px] font-semibold">Response filters</span>
        </div>

        {grantedEndpoints.length === 0 ? (
          <p className="text-[11px] text-muted">Select one or more endpoints.</p>
        ) : userName === null ? (
          <p className="text-[11px] text-muted border border-dashed border-line rounded-[10px] p-3">
            Save the user first, then add response filters -- a filter needs a real user to belong to.
          </p>
        ) : (
          <>
            <div className="shrink-0">
              <SearchInput value={filQuery} onChange={setFilQuery} placeholder="Search filters or fields" />
            </div>
            {(() => {
              const q = filQuery.trim().toLowerCase();
              const matchesFilQuery = (g: FilterGroup) =>
                q === "" ||
                `${g.id} ${g.members.map((m) => `${m.tool} ${m.drop_fields.join(" ")}`).join(" ")}`
                  .toLowerCase()
                  .includes(q);

              const perEndpoint = grantedEndpoints.map((endpoint) => ({
                endpoint,
                groups: groupsFor(endpoint.name).filter(matchesFilQuery),
              }));
              const anyVisible = perEndpoint.some(({ groups }) => groups.length > 0);

              if (q !== "" && !anyVisible) {
                return <p className="text-[11px] text-muted">No filter matches that.</p>;
              }

              return (
                <div className="shrink-0 flex flex-col gap-2.5">
                  {perEndpoint.map(({ endpoint, groups }) => {
                    if (q !== "" && groups.length === 0) return null;
                    const grant = grants.find((g) => g.mcp === endpoint.name)!;
                    const scopedTools = toolsForGrant(endpoint, grant);
                    const expanded = expandedMcps.has(endpoint.name) || q !== "";
                    const shown = expanded ? groups : groups.slice(0, VISIBLE_LIMIT);
                    const hidden = groups.length - shown.length;
                    const addKey = `${endpoint.name}${GROUP_SEP}__new__`;
                    const addOpen = editingKey === addKey;

                    return (
                      <Card key={endpoint.name} className="shrink-0 p-3 flex flex-col gap-2">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="px-1.5 py-0.5 rounded-full text-[10px] font-semibold font-mono bg-well text-muted shrink-0">
                            {endpoint.transport}
                          </span>
                          <button
                            type="button"
                            onClick={onGoInstances}
                            className="text-[12px] font-semibold text-accent border-0 bg-transparent p-0 cursor-pointer truncate text-left hover:underline"
                          >
                            <span>{endpoint.name}</span> ↗
                          </button>
                          <span className="flex-1" />
                          <span className="text-[10.5px] text-muted">
                            {groups.length} {groups.length === 1 ? "filter" : "filters"}
                          </span>
                        </div>

                        {groups.length === 0 && (
                          <p className="text-[11px] text-muted">
                            No response filter exists on this endpoint yet -- the user would see full responses.
                          </p>
                        )}

                        {shown.map((g) => {
                          const key = `${endpoint.name}${GROUP_SEP}${g.id}`;
                          const isEditing = editingKey === key;
                          return isEditing ? (
                            <div key={key} className="p-2.5 flex flex-col gap-2 border border-accent rounded-lg bg-form-soft">
                              <div className="text-[11.5px] font-semibold">{g.id}</div>
                              <AllToolsFieldPicker
                                tools={scopedTools}
                                initial={groupToFieldsByTool(g)}
                                onChange={(fields) => runPlan(endpoint.name, g.id, g.members, fields)}
                                meta={`${endpoint.name} · ${g.id}`}
                              />
                              <div className="flex gap-1.5">
                                <Button variant="secondary" onClick={closeEditors}>
                                  Done
                                </Button>
                                <span className="flex-1" />
                                <Button variant="danger" onClick={() => deleteGroup(g)}>
                                  Delete
                                </Button>
                              </div>
                            </div>
                          ) : (
                            <div
                              key={key}
                              className="p-2.5 flex items-start gap-2.5 border border-border rounded-[9px]"
                            >
                              <div className="flex-1 min-w-0 flex flex-col gap-0.5">
                                <div className="text-[12.5px] font-medium truncate">{g.id}</div>
                                <div className="text-[10.5px] font-mono text-subtle truncate">{groupSummary(g)}</div>
                              </div>
                              <button
                                type="button"
                                aria-label={`Edit ${g.id}`}
                                onClick={() => setEditingKey(key)}
                                className="shrink-0 border border-border rounded-md bg-surface px-2 py-1 cursor-pointer text-[10.5px] font-medium text-body hover:border-accent hover:text-accent"
                              >
                                Edit
                              </button>
                            </div>
                          );
                        })}

                        {!expanded && hidden > 0 && (
                          <button
                            type="button"
                            onClick={() => setExpandedMcps((s) => new Set(s).add(endpoint.name))}
                            className="self-start border-0 bg-transparent cursor-pointer text-[11px] font-medium text-accent p-0"
                          >
                            Show {hidden} more {hidden === 1 ? "filter" : "filters"}
                          </button>
                        )}

                        {addOpen ? (
                          <div className="p-2.5 flex flex-col gap-2 border border-accent rounded-lg bg-form-soft">
                            <Input
                              value={newName}
                              onChange={(e) => setNewName(e.target.value)}
                              placeholder="filter name"
                              required
                            />
                            {nameHasSeparator && (
                              <span className="text-[10.5px] text-danger">
                                Filter names can't contain "{GROUP_SEP}" (reserved for internal grouping).
                              </span>
                            )}
                            <AllToolsFieldPicker
                              tools={scopedTools}
                              onChange={setNewFields}
                              meta={endpoint.name}
                            />
                            {submitError && <p className="text-[10.5px] text-danger">{submitError}</p>}
                            <div className="flex gap-1.5">
                              <Button
                                onClick={() => submitNew(endpoint.name)}
                                disabled={newName.trim() === "" || nameHasSeparator || Object.keys(newFields).length === 0}
                              >
                                Create filter
                              </Button>
                              <Button variant="secondary" onClick={closeEditors}>
                                Cancel
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <button
                            type="button"
                            onClick={() => {
                              closeEditors();
                              setEditingKey(addKey);
                            }}
                            disabled={Object.keys(scopedTools).length === 0}
                            className="flex items-center gap-2 p-2 border border-dashed border-line rounded-[9px] bg-surface cursor-pointer text-left disabled:opacity-50 disabled:cursor-not-allowed"
                          >
                            <span className="w-3.5 h-3.5 rounded border border-dashed border-muted grid place-items-center text-[10px] text-muted">
                              +
                            </span>
                            <span className="text-[11.5px] font-semibold">Add response filter</span>
                          </button>
                        )}
                      </Card>
                    );
                  })}
                </div>
              );
            })()}
          </>
        )}
      </section>
    </div>
  );
}
