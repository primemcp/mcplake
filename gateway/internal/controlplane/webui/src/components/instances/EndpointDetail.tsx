import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { Input } from "../primitives/Input";
import { StatusDot, statusFromString } from "../primitives/StatusDot";
import type { MCPRegistration, RegisterMCPRequest } from "../../api/types";

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
 */
export function EndpointDetail({ endpoint, onUpdate, onRemove }: EndpointDetailProps) {
  const [editing, setEditing] = useState(false);
  const [transport, setTransport] = useState(endpoint.transport);
  const [url, setUrl] = useState(endpoint.connect.url ?? "");
  const [command, setCommand] = useState(endpoint.connect.command ?? "");
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
        transport,
        connect: { url: url || undefined, command: command || undefined },
      });
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col gap-4 p-5 overflow-y-auto">
      <Card className="p-4 flex flex-col gap-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex flex-col gap-0.5 min-w-0">
            <div className="text-base font-semibold">{endpoint.name}</div>
            <div className="text-[11.5px] font-mono text-subtle truncate">
              {endpoint.connect.url || endpoint.connect.command}
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <StatusDot status={statusFromString(endpoint.status)} label={endpoint.status} />
            <Button variant="secondary" onClick={() => setEditing((v) => !v)}>
              {editing ? "Close" : "Edit endpoint"}
            </Button>
          </div>
        </div>

        {editing && (
          <div className="flex flex-col gap-2 p-3 border border-accent rounded-lg bg-accent-soft">
            <div className="flex gap-2">
              <select
                value={transport}
                onChange={(e) => setTransport(e.target.value)}
                className="px-2.5 py-2 border border-border rounded-lg text-[12.5px] font-mono"
              >
                <option value="stdio">stdio</option>
                <option value="sse">sse</option>
                <option value="http">http</option>
              </select>
              <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="url" mono />
            </div>
            <Input
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder="command (stdio)"
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
      </Card>

      <Card className="p-4 flex flex-col gap-2.5">
        <div className="text-[13.5px] font-semibold">Discovered tools</div>
        {tools.length === 0 ? (
          <p className="text-[11.5px] text-subtle">
            No tools discovered yet — the endpoint may still be connecting.
          </p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {tools.map((tool) => (
              <li key={tool.name} className="text-[12px] font-mono text-body">
                {tool.name}
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
