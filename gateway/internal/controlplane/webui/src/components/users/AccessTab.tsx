import { Card } from "../primitives/Card";
import { Toggle } from "../primitives/Toggle";
import { AllToolsFieldPicker } from "../instances/AllToolsFieldPicker";
import type { FieldsByTool } from "../../lib/filterGroups";
import type { Grant, MCPRegistration, ToolSchema } from "../../api/types";
import type { UserFieldsByEndpoint } from "../../api/users";

export type AccessTabProps = {
  endpoints: MCPRegistration[];
  grants: Grant[];
  onGrantsChange: (grants: Grant[]) => void;
  fieldsByEndpoint: UserFieldsByEndpoint;
  onFieldsByEndpointChange: (fields: UserFieldsByEndpoint) => void;
};

function toolsForGrant(endpoint: MCPRegistration, grant: Grant): Record<string, ToolSchema> {
  const all = endpoint.tools ?? {};
  if (grant.tools.includes("*")) return all;
  return Object.fromEntries(Object.entries(all).filter(([name]) => grant.tools.includes(name)));
}

/**
 * Step 1 (endpoint + tool picker) and step 2 (per-endpoint response
 * filters) of the mockup's Access tab. Purely controlled -- this owns no
 * save state of its own, since the detail panel's sticky footer (Save /
 * Discard) commits every tab's edits together; see UserDetail.
 *
 * Grants map straight onto the real `AccessPolicy.grants` shape
 * (`{mcp, tools}`, `tools` supporting the real backend's `"*"` wildcard --
 * unlike FilterPolicy, AccessPolicy actually has one). Step 2 reuses
 * AllToolsFieldPicker exactly as the MCP connections screen does (see
 * ResponseFilterGroup), just scoped down to whichever tools the grant
 * above actually covers, and keyed per endpoint into
 * `UserFieldsByEndpoint` instead of the single-endpoint `FieldsByTool` a
 * connection's own filter editor uses.
 */
export function AccessTab({
  endpoints,
  grants,
  onGrantsChange,
  fieldsByEndpoint,
  onFieldsByEndpointChange,
}: AccessTabProps) {
  const grantedNames = new Set(grants.map((g) => g.mcp));
  const grantedEndpoints = endpoints.filter((e) => grantedNames.has(e.name));

  const setGrantTools = (mcp: string, tools: string[]) => {
    onGrantsChange(grants.map((g) => (g.mcp === mcp ? { ...g, tools } : g)));
  };

  const toggleEndpoint = (mcp: string) => {
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

  if (endpoints.length === 0) {
    return <p className="text-[11px] text-muted px-1">No MCP connections registered yet.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <section className="flex flex-col gap-2">
        <div className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">
          Step 1 · Endpoints and tools
        </div>
        <div className="flex flex-col gap-1.5">
          {endpoints.map((endpoint) => {
            const grant = grants.find((g) => g.mcp === endpoint.name);
            const allTools = Object.keys(endpoint.tools ?? {});
            const isAllTools = grant?.tools.includes("*") ?? true;
            const checkedTools = new Set(grant ? (isAllTools ? allTools : grant.tools) : []);

            return (
              <Card key={endpoint.name} className="p-2.5 flex flex-col gap-2">
                <div className="flex items-center gap-2.5">
                  <Toggle
                    checked={grant !== undefined}
                    onChange={() => toggleEndpoint(endpoint.name)}
                    aria-label={`Grant ${endpoint.name}`}
                  />
                  <span className="text-[12.5px] font-medium flex-1">{endpoint.name}</span>
                  <span className="text-[10.5px] text-muted">
                    {allTools.length} {allTools.length === 1 ? "tool" : "tools"}
                  </span>
                </div>
                {grant && allTools.length > 0 && (
                  <div className="pl-[38px] flex flex-col gap-1.5">
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

      {grantedEndpoints.length > 0 && (
        <section className="flex flex-col gap-2">
          <div className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">
            Step 2 · Response filters
          </div>
          <div className="flex flex-col gap-2.5">
            {grantedEndpoints.map((endpoint) => {
              const grant = grants.find((g) => g.mcp === endpoint.name)!;
              const scopedTools = toolsForGrant(endpoint, grant);
              return (
                <Card key={endpoint.name} className="p-3 flex flex-col gap-2">
                  <AllToolsFieldPicker
                    tools={scopedTools}
                    initial={fieldsByEndpoint[endpoint.name] ?? {}}
                    onChange={(fields) => setEndpointFields(endpoint.name, fields)}
                    meta={endpoint.name}
                  />
                </Card>
              );
            })}
          </div>
        </section>
      )}
    </div>
  );
}
