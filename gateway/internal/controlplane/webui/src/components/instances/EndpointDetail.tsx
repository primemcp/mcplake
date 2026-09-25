import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { statusFromString, statusTextColor } from "../primitives/StatusDot";
import type { MCPRegistration, MCPTransport, RegisterMCPRequest } from "../../api/types";
import {
  asTransport,
  buildConnect,
  connectFieldsComplete,
  endpointTarget,
  errorField,
  type ConnectFields,
} from "../../lib/transport";
import { ConnectFieldsEditor } from "./ConnectFieldsEditor";

export type EndpointDetailProps = {
  endpoint: MCPRegistration;
  onUpdate: (req: RegisterMCPRequest) => Promise<void>;
  onRemove: (name: string) => Promise<void>;
  onSetEnabled: (name: string, enabled: boolean) => Promise<void>;
};

/**
 * There is no PUT for MCPs in the real admin API (see #77's design doc) —
 * "edit" re-registers with the same name, which the backend treats as an
 * upsert. `enabled` is passed through explicitly on every save (not left
 * to the request's default-to-true) so saving an unrelated command/args
 * edit on a disabled endpoint doesn't silently re-enable it.
 *
 * The Enabled/Disabled toggle now wires to the real
 * `PATCH /admin/mcps/:name` (#153) -- a reversible soft-disable, distinct
 * from "Delete endpoint" (DELETE, unrecoverable: drops the registration
 * and its schema cache along with every filter/grant on it).
 *
 * Delete sits in the header beside Edit and the toggle, with its own
 * two-step confirm (#202). It used to live at the bottom of the collapsed
 * edit panel, where operators reported they could not find it at all -- and
 * a failed delete there reset the button without a word, so the two read
 * the same. Its failures now land in the same error slot as the toggle's.
 *
 * Transport is editable since ADR-0017 implemented http and sse; it used to
 * be pinned to "stdio" on save because the backend rejected anything else.
 * Changing it is a real operation, not a display setting: an upsert that
 * re-registers the same name against a different kind of downstream.
 */
export function EndpointDetail({ endpoint, onUpdate, onRemove, onSetEnabled }: EndpointDetailProps) {
  const [editing, setEditing] = useState(false);
  const [transport, setTransport] = useState<MCPTransport>(asTransport(endpoint.transport));
  const [fields, setFields] = useState<ConnectFields>({
    command: endpoint.connect.command ?? "",
    args: (endpoint.connect.arguments ?? []).join(" "),
    url: endpoint.connect.url ?? "",
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [removing, setRemoving] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [togglingEnabled, setTogglingEnabled] = useState(false);

  const tools = Object.values(endpoint.tools ?? {});

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      await onUpdate({
        name: endpoint.name,
        transport,
        connect: {
          ...buildConnect(transport, fields),
          // Not editable here (no env UI yet) -- carried through explicitly
          // so saving a command/args edit never silently wipes it.
          env: endpoint.connect.env,
        },
        // Passed through explicitly (not left to the request's
        // default-to-true) so saving an unrelated command/url edit on a
        // disabled endpoint doesn't silently re-enable it.
        enabled: endpoint.enabled,
      });
      setEditing(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setSaving(false);
    }
  };

  const toggleEnabled = async () => {
    setTogglingEnabled(true);
    setError(null);
    try {
      await onSetEnabled(endpoint.name, !endpoint.enabled);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update.");
    } finally {
      setTogglingEnabled(false);
    }
  };

  const requestDelete = async () => {
    setRemoving(true);
    setError(null);
    try {
      await onRemove(endpoint.name);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete.");
    } finally {
      setRemoving(false);
      setConfirmDelete(false);
    }
  };

  const status = statusFromString(endpoint.status);
  // A failed save is usually about the url or the command; show the server's
  // complaint against that field rather than as a banner the operator has to
  // map back onto the form themselves.
  const field = error ? errorField(error) : null;

  return (
    <div className="flex flex-col gap-4 p-5 overflow-y-auto">
      <Card className="overflow-hidden">
        <div className="p-[14px_16px] border-b border-border-soft flex flex-wrap items-center gap-[10px_14px]">
          <div className="flex-[1_1_240px] min-w-0 flex flex-col gap-[3px]">
            <div className="text-base font-semibold tracking-tight">{endpoint.name}</div>
            {/* The command line for stdio, the URL for anything remote --
                the same slot, because it answers the same question: what is
                on the other end of this registration. */}
            <div className="text-[11.5px] font-mono text-subtle truncate">
              {endpointTarget(endpoint)}
            </div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <button
              type="button"
              onClick={() => {
                setEditing((v) => !v);
                setConfirmDelete(false);
              }}
              className="flex items-center gap-1.5 px-[11px] py-[6px] border border-border rounded-[9px] bg-surface cursor-pointer text-xs font-medium text-body hover:border-accent hover:text-accent"
            >
              {editing ? "Close" : "Edit endpoint"}
            </button>
            <button
              type="button"
              onClick={() => {
                setConfirmDelete(true);
                setEditing(false);
              }}
              disabled={removing || confirmDelete}
              className="flex items-center gap-1.5 px-[11px] py-[6px] border border-border rounded-[9px] bg-surface cursor-pointer text-xs font-medium text-body hover:border-danger hover:text-danger disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Delete endpoint
            </button>
            <button
              type="button"
              role="switch"
              aria-checked={endpoint.enabled}
              aria-label={`${endpoint.enabled ? "Disable" : "Enable"} ${endpoint.name}`}
              disabled={togglingEnabled}
              onClick={toggleEnabled}
              className={`flex items-center gap-2 py-[5px] pl-2 pr-2.5 rounded-full border border-border cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed ${
                endpoint.enabled ? "bg-success-soft" : "bg-danger-bg"
              }`}
            >
              <span
                className={`flex w-[30px] h-[17px] rounded-full p-0.5 ${
                  endpoint.enabled ? "justify-end bg-success" : "justify-start bg-danger"
                }`}
              >
                <span className="w-[13px] h-[13px] rounded-full bg-white" />
              </span>
              <span className={`text-[11.5px] font-semibold ${endpoint.enabled ? "text-success" : "text-danger"}`}>
                {endpoint.enabled ? "Enabled" : "Disabled"}
              </span>
            </button>
          </div>
        </div>

        {error && field === null && <p className="text-[10.5px] text-danger px-4 pt-2.5">{error}</p>}

        {confirmDelete && (
          <div className="p-[12px_16px] border-b border-border-soft bg-danger-bg flex flex-wrap items-center gap-2">
            <p className="flex-[1_1_240px] m-0 text-[11.5px] text-body">
              Delete <span className="font-mono font-semibold">{endpoint.name}</span>? This removes the
              endpoint, its filters and every grant on it — for all users.
            </p>
            <Button variant="danger" onClick={requestDelete} disabled={removing}>
              Confirm delete
            </Button>
            <Button variant="secondary" onClick={() => setConfirmDelete(false)} disabled={removing}>
              Cancel
            </Button>
          </div>
        )}

        {editing && (
          <div className="p-[14px_16px] border-b border-border-soft bg-form-soft flex flex-col gap-2">
            <ConnectFieldsEditor
              transport={transport}
              onTransportChange={setTransport}
              fields={fields}
              onFieldsChange={setFields}
              errorField={field}
              errorMessage={error}
              disabled={saving}
            />
            <div className="flex flex-wrap items-center gap-1.5">
              <Button onClick={save} disabled={saving || !connectFieldsComplete(transport, fields)}>
                Save endpoint
              </Button>
              <Button variant="secondary" onClick={() => setEditing(false)}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        <div className="p-[12px_16px] flex flex-wrap items-center gap-[8px_14px]">
          <span className={`text-[11.5px] font-medium ${endpoint.enabled ? statusTextColor[status] : "text-danger"}`}>
            {endpoint.enabled
              ? endpoint.status
              : "Disabled — requests to this endpoint are rejected and grants are suspended"}
          </span>
          <span className="flex-1" />
          <span className="text-[11.5px] text-subtle font-mono">{endpoint.transport}</span>
          <span className="text-[11.5px] text-subtle">
            {tools.length} {tools.length === 1 ? "tool" : "tools"}
          </span>
        </div>
      </Card>
    </div>
  );
}
