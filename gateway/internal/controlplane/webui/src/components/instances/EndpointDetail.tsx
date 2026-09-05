import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { Input } from "../primitives/Input";
import { statusFromString, statusTextColor } from "../primitives/StatusDot";
import type { MCPRegistration, RegisterMCPRequest } from "../../api/types";
import { TransportPicker } from "./TransportPicker";

export type EndpointDetailProps = {
  endpoint: MCPRegistration;
  onUpdate: (req: RegisterMCPRequest) => Promise<void>;
  onRemove: (name: string) => Promise<void>;
};

/**
 * There is no PUT/PATCH for MCPs in the real admin API (see #77's design
 * doc) — "edit" re-registers with the same name, which the backend treats
 * as an upsert.
 *
 * The mockup's Enabled/Disabled toggle IS fully specified there (colors,
 * layout) and is reproduced here exactly -- but the real backend has no
 * soft-disable state (only registered/not, via DELETE /admin/mcps/:name),
 * so switching it to "Disabled" is, honestly, unregistering the endpoint
 * (confirmed first) rather than a reversible pause. There's no "Enabled"
 * direction to wire up after that, since the endpoint is simply gone.
 *
 * Transport is fixed to stdio when saving: cache.Registry.Register on the
 * real backend rejects anything else with "unsupported transport (only
 * stdio is implemented)" — sse/http aren't wired up in the mcp package
 * yet. TransportPicker shows all three explicitly, with sse/http visibly
 * disabled, rather than hiding the fact that they exist but don't work —
 * an explicit UX request over silently defaulting in code. A URL field is
 * still omitted; unlike transport, the DTO having a `url` slot doesn't
 * make showing it as a live option honest when nothing consumes it.
 */
export function EndpointDetail({ endpoint, onUpdate, onRemove }: EndpointDetailProps) {
  const [editing, setEditing] = useState(false);
  const [command, setCommand] = useState(endpoint.connect.command ?? "");
  const [args, setArgs] = useState((endpoint.connect.arguments ?? []).join(" "));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [removing, setRemoving] = useState(false);

  const tools = Object.values(endpoint.tools ?? {});

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      await onUpdate({
        name: endpoint.name,
        transport: "stdio",
        connect: {
          command,
          arguments: args.trim() === "" ? undefined : args.trim().split(/\s+/),
        },
      });
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setSaving(false);
    }
  };

  const status = statusFromString(endpoint.status);

  return (
    <div className="flex flex-col gap-4 p-5 overflow-y-auto">
      <Card className="overflow-hidden">
        <div className="p-[14px_16px] border-b border-border-soft flex flex-wrap items-center gap-[10px_14px]">
          <div className="flex-[1_1_240px] min-w-0 flex flex-col gap-[3px]">
            <div className="text-base font-semibold tracking-tight">{endpoint.name}</div>
            <div className="text-[11.5px] font-mono text-subtle truncate">
              {endpoint.connect.command}
              {endpoint.connect.arguments?.length ? ` ${endpoint.connect.arguments.join(" ")}` : ""}
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <button
              type="button"
              onClick={() => setEditing((v) => !v)}
              className="flex items-center gap-1.5 px-[11px] py-[6px] border border-border rounded-[9px] bg-surface cursor-pointer text-xs font-medium text-body hover:border-accent hover:text-accent"
            >
              {editing ? "Close" : "Edit endpoint"}
            </button>
            <button
              type="button"
              disabled={removing}
              onClick={async () => {
                if (!window.confirm(`Remove ${endpoint.name}? There is no soft-disable — this unregisters it.`)) {
                  return;
                }
                setRemoving(true);
                await onRemove(endpoint.name);
              }}
              className="flex items-center gap-2 py-[5px] pl-2 pr-2.5 rounded-full border border-border cursor-pointer bg-success-soft"
            >
              <span className="flex w-[30px] h-[17px] rounded-full p-0.5 justify-end bg-success">
                <span className="w-[13px] h-[13px] rounded-full bg-white" />
              </span>
              <span className="text-[11.5px] font-semibold text-success">Enabled</span>
            </button>
          </div>
        </div>

        {editing && (
          <div className="p-[14px_16px] border-b border-border-soft bg-form-soft flex flex-col gap-2">
            <TransportPicker />
            <Input value={command} onChange={(e) => setCommand(e.target.value)} placeholder="command" mono />
            <Input
              value={args}
              onChange={(e) => setArgs(e.target.value)}
              placeholder="arguments (space-separated)"
              mono
            />
            {error && <p className="text-[10.5px] text-danger">{error}</p>}
            <div className="flex gap-1.5">
              <Button onClick={save} disabled={saving}>
                Save endpoint
              </Button>
              <Button variant="secondary" onClick={() => setEditing(false)}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        <div className="p-[12px_16px] flex flex-wrap items-center gap-[8px_14px]">
          <span className={`text-[11.5px] font-medium ${statusTextColor[status]}`}>{endpoint.status}</span>
          <span className="flex-1" />
          <span className="text-[11.5px] text-subtle">stdio</span>
          <span className="text-[11.5px] text-subtle">
            {tools.length} {tools.length === 1 ? "tool" : "tools"}
          </span>
        </div>
      </Card>
    </div>
  );
}
