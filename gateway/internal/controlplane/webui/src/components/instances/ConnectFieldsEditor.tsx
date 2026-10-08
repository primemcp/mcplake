import type { ReactNode } from "react";
import { Input } from "../primitives/Input";
import type { MCPTransport } from "../../api/types";
import { isRemoteTransport, type ConnectFields } from "../../lib/transport";
import { TransportPicker } from "./TransportPicker";

export type ConnectFieldsEditorProps = {
  transport: MCPTransport;
  onTransportChange: (transport: MCPTransport) => void;
  fields: ConnectFields;
  onFieldsChange: (fields: ConnectFields) => void;
  /** The field the last failed save complained about, if any. */
  errorField?: "url" | "command" | null;
  errorMessage?: string | null;
  disabled?: boolean;
  /** Renders each field; the two forms label them differently. */
  labelled?: boolean;
};

function Field({
  label,
  labelled,
  children,
}: {
  label: string;
  labelled: boolean;
  children: ReactNode;
}) {
  if (!labelled) return <>{children}</>;
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-[11px] font-semibold text-subtle">{label}</span>
      {children}
    </label>
  );
}

/**
 * The transport picker plus whichever connect fields that transport actually
 * uses -- shared by the add and edit forms so the two cannot disagree about
 * what an http endpoint needs.
 *
 * Command/args and url are all held in the caller's state at once, even
 * though only one set is ever shown: flipping stdio -> http -> stdio while
 * deciding must not silently eat what you already typed. `buildConnect` is
 * what drops the unused half at the request boundary.
 */
export function ConnectFieldsEditor({
  transport,
  onTransportChange,
  fields,
  onFieldsChange,
  errorField = null,
  errorMessage = null,
  disabled = false,
  labelled = false,
}: ConnectFieldsEditorProps) {
  const set = (patch: Partial<ConnectFields>) => onFieldsChange({ ...fields, ...patch });
  const remote = isRemoteTransport(transport);
  const fieldError = (name: "url" | "command") =>
    errorField === name && errorMessage ? errorMessage : null;

  return (
    <>
      <TransportPicker value={transport} onChange={onTransportChange} disabled={disabled} />

      {remote ? (
        <>
          <Field label="URL" labelled={labelled}>
            <Input
              value={fields.url}
              onChange={(e) => set({ url: e.target.value })}
              placeholder={transport === "sse" ? "https://mcp.example.com/sse" : "https://mcp.example.com/mcp"}
              aria-label={labelled ? undefined : "URL"}
              aria-invalid={fieldError("url") !== null}
              disabled={disabled}
              mono
            />
          </Field>
          {fieldError("url") ? (
            <p className="text-[10.5px] text-danger">{fieldError("url")}</p>
          ) : (
            // Stated up front rather than discovered by submitting: the
            // gateway requires https, or http only on a loopback host
            // (mcp.ValidateEndpointURL). Deliberately a hint and not a
            // client-side check -- duplicating a security rule in TypeScript
            // is how the two drift, and the server stays the one place that
            // decides. See ADR-0017.
            <p className="text-[10.5px] text-muted">
              Must be https, or http only on a loopback host (localhost, 127.0.0.1, ::1).
            </p>
          )}
        </>
      ) : (
        <>
          <Field label="Command" labelled={labelled}>
            <Input
              value={fields.command}
              onChange={(e) => set({ command: e.target.value })}
              placeholder="mcp-server-postgres"
              aria-label={labelled ? undefined : "Command"}
              aria-invalid={fieldError("command") !== null}
              disabled={disabled}
              mono
            />
          </Field>
          {fieldError("command") && (
            <p className="text-[10.5px] text-danger">{fieldError("command")}</p>
          )}
          <Field label="Arguments (optional, space-separated)" labelled={labelled}>
            <Input
              value={fields.args}
              onChange={(e) => set({ args: e.target.value })}
              placeholder="--read-only --db mcp"
              aria-label={labelled ? undefined : "Arguments (optional, space-separated)"}
              disabled={disabled}
              mono
            />
          </Field>
        </>
      )}
    </>
  );
}
