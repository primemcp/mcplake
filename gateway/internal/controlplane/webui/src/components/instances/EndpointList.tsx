import { useMemo, useState } from "react";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { Modal } from "../primitives/Modal";
import { SearchInput } from "../primitives/SearchInput";
import { statusFromString, statusTextColor } from "../primitives/StatusDot";
import type { MCPRegistration } from "../../api/types";
import { AddEndpointForm } from "./AddEndpointForm";

const dotColor: Record<string, string> = {
  active: "bg-success",
  connecting: "bg-accent",
  unreachable: "bg-danger",
  unknown: "bg-muted",
};

export type EndpointListProps = {
  endpoints: MCPRegistration[];
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  selectedName: string | null;
  onSelect: (name: string) => void;
  onCreate: (name: string, command: string, args: string[]) => Promise<void>;
};

export function EndpointList({
  endpoints,
  loading,
  error,
  onRetry,
  selectedName,
  onSelect,
  onCreate,
}: EndpointListProps) {
  const [query, setQuery] = useState("");
  const [adding, setAdding] = useState(false);

  const hits = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === "") return endpoints;
    return endpoints.filter(
      (e) =>
        e.name.toLowerCase().includes(q) ||
        (e.connect.url ?? "").toLowerCase().includes(q) ||
        (e.connect.command ?? "").toLowerCase().includes(q),
    );
  }, [endpoints, query]);

  return (
    <div className="w-[320px] shrink-0 border-r border-border bg-surface p-3 flex flex-col gap-2.5 overflow-hidden">
      <SearchInput value={query} onChange={setQuery} placeholder="Search connections, URLs" />

      {error ? (
        <ErrorNotice onRetry={onRetry} />
      ) : loading ? (
        <p className="text-[11px] text-muted px-1">Loading…</p>
      ) : (
        <>
          <div className="text-[10.5px] text-muted px-1">
            {hits.length} of {endpoints.length}
          </div>
          <div className="flex-1 min-h-[90px] overflow-y-auto flex flex-col gap-2">
            {hits.length === 0 && (
              <div className="p-3 border border-dashed border-border rounded-[10px] text-[11.5px] text-subtle">
                {endpoints.length === 0 ? "No MCP endpoints yet." : "No connection matches that."}
              </div>
            )}
            {hits.map((e) => {
              const status = statusFromString(e.status);
              const selected = e.name === selectedName;
              const toolCount = Object.keys(e.tools ?? {}).length;
              return (
                <button
                  key={e.name}
                  type="button"
                  onClick={() => onSelect(e.name)}
                  className={`text-left p-[11px_12px] border rounded-[11px] cursor-pointer flex flex-col gap-[3px] ${
                    selected ? "border-accent bg-accent-soft" : "border-border bg-surface"
                  }`}
                >
                  <div className="flex items-center gap-[7px] min-w-0">
                    <span className={`w-[7px] h-[7px] rounded-full shrink-0 ${dotColor[status]}`} />
                    <span className="text-[12.5px] font-semibold truncate">{e.name}</span>
                  </div>
                  <div className="text-[10.5px] font-mono text-subtle truncate">
                    {e.connect.command || "—"}
                  </div>
                  <div className={`text-[10.5px] font-medium ${statusTextColor[status]}`}>{e.status}</div>
                  <div className="text-[10.5px] text-subtle">
                    stdio · {toolCount} {toolCount === 1 ? "tool" : "tools"}
                  </div>
                </button>
              );
            })}
          </div>
        </>
      )}

      <button
        type="button"
        onClick={() => setAdding(true)}
        className="flex items-center gap-2.5 p-3 border border-dashed border-border rounded-[10px] bg-surface cursor-pointer text-left"
      >
        <span className="w-4 h-4 rounded border border-dashed border-muted grid place-items-center text-[11px] text-muted">
          +
        </span>
        <span className="flex flex-col gap-0.5">
          <span className="text-[12.5px] font-semibold">Add MCP endpoint</span>
          <span className="text-[10.5px] text-muted">a command is all the gateway needs</span>
        </span>
      </button>

      <Modal open={adding} onClose={() => setAdding(false)} title="Add MCP endpoint">
        <AddEndpointForm
          onCancel={() => setAdding(false)}
          onCreate={async (name, command, args) => {
            await onCreate(name, command, args);
            setAdding(false);
          }}
        />
      </Modal>
    </div>
  );
}
