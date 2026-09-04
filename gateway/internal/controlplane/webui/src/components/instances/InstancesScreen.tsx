import { useEffect, useState } from "react";
import { EmptyState } from "../primitives/EmptyState";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { useEndpoints, findEndpoint } from "../../api/endpoints";
import { useFilterPolicies } from "../../api/policies";
import { EndpointDetail } from "./EndpointDetail";
import { EndpointList } from "./EndpointList";
import { ResponseFilterGroup } from "./ResponseFilterGroup";

export function InstancesScreen() {
  const { endpoints, loading, error, retry, register, unregister } = useEndpoints();
  const filterPolicies = useFilterPolicies();
  const [selectedName, setSelectedName] = useState<string | null>(null);

  // Keep the selection valid as the list changes (e.g. after a delete).
  useEffect(() => {
    if (selectedName === null && endpoints.length > 0) {
      setSelectedName(endpoints[0].name);
    } else if (selectedName !== null && !findEndpoint(endpoints, selectedName)) {
      setSelectedName(endpoints[0]?.name ?? null);
    }
  }, [endpoints, selectedName]);

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
          onSelect={setSelectedName}
          onCreate={async (name, url) => {
            await register({ name, connect: { url } });
            setSelectedName(name);
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
