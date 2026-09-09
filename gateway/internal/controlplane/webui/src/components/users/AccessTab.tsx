import { useState } from "react";
import { Card } from "../primitives/Card";
import { SearchInput } from "../primitives/SearchInput";
import { statusFromString } from "../primitives/StatusDot";
import { Toggle } from "../primitives/Toggle";
import { AllToolsFieldPicker } from "../instances/AllToolsFieldPicker";
import type { FieldsByTool } from "../../lib/filterGroups";
import type { FilterPolicy, Grant, MCPRegistration, ToolSchema } from "../../api/types";
import type { UserFieldsByEndpoint } from "../../api/users";

export type AccessTabProps = {
  endpoints: MCPRegistration[];
  grants: Grant[];
  onGrantsChange: (grants: Grant[]) => void;
  fieldsByEndpoint: UserFieldsByEndpoint;
  onFieldsByEndpointChange: (fields: UserFieldsByEndpoint) => void;
  /** Every real FilterPolicy in the system -- offered in step 2 as
   * "start from an existing filter" templates (ADR-0010: clone, never
   * link -- ADR-0002's ClaimMatcher is AND-only, so one filter literally
   * cannot correctly serve two users with different match conditions). */
  allFilters: FilterPolicy[];
  onGoInstances: () => void;
};

type ConnFilter = "all" | "granted" | "connected" | "pending";

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

/**
 * Step 1 (endpoint picker) and step 2 (per-endpoint response filters) of
 * the mockup's Access tab. Purely controlled -- this owns no save state of
 * its own, since the detail panel's sticky footer (Save / Discard) commits
 * every tab's edits together; see UserDetail.
 *
 * Fetched the real mockup source via DesignSync to match this structurally
 * (numbered steps, search, a connection-status filter row, a checkbox-mark
 * per endpoint instead of a plain toggle, an "open in MCP connections"
 * link, response filters grouped by endpoint) rather than the flatter,
 * guessed-at layout an earlier pass had. Two disclosed differences from
 * that same source:
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
 * Step 2 reuses AllToolsFieldPicker exactly as the MCP connections screen
 * does (see ResponseFilterGroup), scoped to whichever tools the grant
 * covers. The mockup's Step 2 lets you pick from a shared pool of
 * existing filters -- the real backend has no such pool (a FilterPolicy
 * belongs to no one but itself; see ADR-0010), so "start from an existing
 * filter" here clones another filter's `drop_fields` as a one-time
 * starting point instead of a live link.
 */
export function AccessTab({
  endpoints,
  grants,
  onGrantsChange,
  fieldsByEndpoint,
  onFieldsByEndpointChange,
  allFilters,
  onGoInstances,
}: AccessTabProps) {
  const [epQuery, setEpQuery] = useState("");
  const [connFilter, setConnFilter] = useState<ConnFilter>("all");
  // Bumped per endpoint whenever a template is applied, forced into
  // AllToolsFieldPicker's key -- its `initial` prop is only ever read on
  // mount, so picking a template (which changes fieldsByEndpoint out from
  // under an already-mounted picker) needs a remount to actually show up.
  const [templateBump, setTemplateBump] = useState<Record<string, number>>({});

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
      if (mcp in fieldsByEndpoint) {
        const rest = { ...fieldsByEndpoint };
        delete rest[mcp];
        onFieldsByEndpointChange(rest);
      }
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

  const setEndpointFields = (mcp: string, fields: FieldsByTool) => {
    onFieldsByEndpointChange({ ...fieldsByEndpoint, [mcp]: fields });
  };

  const applyTemplate = (mcp: string, template: FilterPolicy) => {
    const current = fieldsByEndpoint[mcp] ?? {};
    setEndpointFields(mcp, { ...current, [template.tool]: template.drop_fields });
    setTemplateBump((b) => ({ ...b, [mcp]: (b[mcp] ?? 0) + 1 }));
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

  if (endpoints.length === 0) {
    return <p className="text-[11px] text-muted px-1">No MCP connections registered yet.</p>;
  }

  return (
    <div className="h-full min-h-0 grid grid-cols-2 gap-px bg-border-soft border border-border-soft rounded-lg overflow-hidden">
      {/* Step 1's own header/filters/search stay put -- only the endpoint
          list below them scrolls, matching the mockup's own nested
          overflow-y-auto around just its `epCards` list, not the whole
          column. Step 2 (the other grid column) scrolls as one unit
          instead, also per the mockup. Both are independent regions, not
          one page-level scroll -- the ask this replaces (a single long
          page) came from an earlier stacked-sections layout that never
          matched the mockup's actual 2-column grid. */}
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
              <Card key={endpoint.name} className="overflow-hidden">
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
        ) : (
          <div className="flex flex-col gap-2.5">
            {grantedEndpoints.map((endpoint) => {
              const grant = grants.find((g) => g.mcp === endpoint.name)!;
              const scopedTools = toolsForGrant(endpoint, grant);
              const templates = allFilters.filter(
                (f) => f.mcp === endpoint.name && Object.keys(scopedTools).includes(f.tool),
              );

              return (
                <Card key={endpoint.name} className="p-3 flex flex-col gap-2">
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
                  </div>

                  {templates.length > 0 && (
                    <label className="flex items-center gap-2 text-[10.5px] text-muted">
                      Start from
                      <select
                        aria-label={`Start from an existing filter on ${endpoint.name}`}
                        value=""
                        onChange={(e) => {
                          const src = templates.find((f) => f.name === e.target.value);
                          if (src) applyTemplate(endpoint.name, src);
                        }}
                        className="flex-1 min-w-0 border border-border rounded-md bg-surface text-[10.5px] py-1 px-1.5"
                      >
                        <option value="" disabled>
                          an existing filter…
                        </option>
                        {templates.map((f) => (
                          <option key={f.name} value={f.name}>
                            {f.name} · {f.tool} · {f.drop_fields.length} fields
                          </option>
                        ))}
                      </select>
                    </label>
                  )}

                  <AllToolsFieldPicker
                    key={templateBump[endpoint.name] ?? 0}
                    tools={scopedTools}
                    initial={fieldsByEndpoint[endpoint.name] ?? {}}
                    onChange={(fields) => setEndpointFields(endpoint.name, fields)}
                    meta={endpoint.name}
                  />
                </Card>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
