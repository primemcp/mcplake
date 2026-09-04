import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { Input } from "../primitives/Input";
import { statusFromString, statusTextColor } from "../primitives/StatusDot";
import type { MCPRegistration, RegisterMCPRequest } from "../../api/types";
import { SchemaFieldPicker } from "./SchemaFieldPicker";
import { TransportPicker } from "./TransportPicker";

export type EndpointDetailProps = {
  endpoint: MCPRegistration;
  onUpdate: (req: RegisterMCPRequest) => Promise<void>;
  onRemove: (name: string) => Promise<void>;
};

/**
 * There is no PUT/PATCH for MCPs in the real admin API (see #77's design
 * doc) — "edit" re-registers with the same name, which the backend treats
 * as an upsert. There's likewise no soft enable/disable: the mockup's
 * toggle doesn't map onto anything the backend supports, so "removing" an
 * endpoint here means unregistering it (DELETE /admin/mcps/:name), not
 * suspending it.
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
          <button
            type="button"
            onClick={() => setEditing((v) => !v)}
            className="flex items-center gap-1.5 px-[11px] py-[6px] border border-border rounded-[9px] bg-surface cursor-pointer shrink-0 text-xs font-medium text-body hover:border-accent hover:text-accent"
          >
            {editing ? "Close" : "Edit endpoint"}
          </button>
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
              <span className="flex-1" />
              <Button
                variant="danger"
                disabled={removing}
                onClick={async () => {
                  setRemoving(true);
                  await onRemove(endpoint.name);
                }}
              >
                Remove endpoint
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

      <Card className="p-4 flex flex-col gap-2.5">
        <div className="flex flex-wrap items-baseline gap-2.5">
          <div className="text-[13.5px] font-semibold">Discovered tools</div>
          <div className="text-[11.5px] text-muted">
            input parameters — response fields are in Response filters below
          </div>
        </div>
        {tools.length === 0 ? (
          <p className="text-[11.5px] text-subtle">
            No tools discovered yet — the endpoint may still be connecting.
          </p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {tools.map((tool) => (
              <li key={tool.name} className="p-2.5 border border-border rounded-[10px] flex flex-col gap-1.5">
                <div className="text-[12.5px] font-semibold">{tool.name}</div>
                <SchemaFieldPicker schema={tool.input_schema} />
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
