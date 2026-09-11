import { useEffect, useState } from "react";
import { EmptyState } from "../primitives/EmptyState";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { useEndpoints, findEndpoint } from "../../api/endpoints";
import { useFilterPolicies } from "../../api/policies";
import { clearStorage, readStorage, writeStorage } from "../../lib/storage";
import { EndpointDetail } from "./EndpointDetail";
import { EndpointList } from "./EndpointList";
import { ResponseFilterGroup } from "./ResponseFilterGroup";

const SELECTED_KEY = "mcplake:instances:selected";
const isString = (v: unknown): v is string => typeof v === "string";

export function InstancesScreen() {
  const { endpoints, loading, error, retry, register, unregister } = useEndpoints();
  const filterPolicies = useFilterPolicies();
  const [selectedName, setSelectedName] = useState<string | null>(() => readStorage(SELECTED_KEY, isString));

  const selectEndpoint = (name: string | null) => {
    setSelectedName(name);
    if (name === null) clearStorage(SELECTED_KEY);
    else writeStorage(SELECTED_KEY, name);
  };

  // Keep the selection valid as the list changes (e.g. after a delete).
  // Gated on `loading` so a selection just restored from localStorage
  // isn't immediately treated as "not found" and discarded while
  // `endpoints` is still an empty placeholder waiting on the first fetch.
  useEffect(() => {
    if (loading) return;
    if (selectedName === null && endpoints.length > 0) {
      selectEndpoint(endpoints[0].name);
    } else if (selectedName !== null && !findEndpoint(endpoints, selectedName)) {
      selectEndpoint(endpoints[0]?.name ?? null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [endpoints, selectedName, loading]);

  const selected = selectedName ? findEndpoint(endpoints, selectedName) : undefined;

  return (
    <div className="flex-1 min-h-0 flex flex-col">
      <div className="px-6 pt-4.5 pb-3.5 bg-surface border-b border-border flex items-end justify-between gap-5">
        <div className="flex flex-col gap-0.5">
          <h1 className="m-0 text-[19px] font-semibold tracking-tight">MCP connections</h1>
          <p className="m-0 text-[12.5px] text-subtle">
            Every endpoint the gateway talks to, and each one's response filters.
          </p>
        </div>
        <div className="text-xs text-subtle">{endpoints.length} connected</div>
      </div>

      <div className="flex-1 min-h-0 flex">
        <EndpointList
          endpoints={endpoints}
          loading={loading}
          error={error}
          onRetry={retry}
          selectedName={selectedName}
          onSelect={selectEndpoint}
          onCreate={async (name, command, args) => {
            await register({
              name,
              transport: "stdio",
              connect: { command, arguments: args.length > 0 ? args : undefined },
            });
            selectEndpoint(name);
          }}
        />

        <div className="flex-1 min-w-0 overflow-y-auto">
          {error ? (
            <div className="p-5">
              <ErrorNotice onRetry={retry} />
            </div>
          ) : selected ? (
            <>
              <EndpointDetail
                endpoint={selected}
                onUpdate={(req) => register(req)}
                onRemove={async (name) => {
                  await unregister(name);
                }}
              />
              <div className="px-5 pb-5 -mt-2">
                <ResponseFilterGroup
                  // Without a key, switching the selected endpoint doesn't
                  // remount this (React just re-renders with new props),
                  // so its in-progress form state -- including
                  // AllToolsFieldPicker's toggled fields and inferred
                  // active tool -- leaked into whichever endpoint you
                  // switched to next. A tool name scoped from the
                  // previous endpoint doesn't exist on the new one, so
                  // the field list collapsed to zero rows and "Create
                  // filter" would have submitted a tool that isn't even
                  // registered on the new endpoint.
                  key={selected.name}
                  endpoint={selected}
                  filters={filterPolicies.filterPolicies}
                  loading={filterPolicies.loading}
                  error={filterPolicies.error}
                  onRetry={filterPolicies.retry}
                  onCreate={(name, tool, dropFields) =>
                    filterPolicies.create({
                      name,
                      match: [],
                      mcp: selected.name,
                      tool,
                      drop_fields: dropFields,
                    })
                  }
                  onUpdate={(name, tool, dropFields) =>
                    filterPolicies.update(name, {
                      name,
                      match: [],
                      mcp: selected.name,
                      tool,
                      drop_fields: dropFields,
                    })
                  }
                  onDelete={(name) => filterPolicies.remove(name)}
                />
              </div>
            </>
          ) : (
            !loading && (
              <div className="flex items-center justify-center h-full p-6">
                <EmptyState
                  title="No MCP endpoints yet"
                  subtitle="Add one from the left to get started."
                />
              </div>
            )
          )}
        </div>
      </div>
    </div>
  );
}
