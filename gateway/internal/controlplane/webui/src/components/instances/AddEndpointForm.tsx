import { useState } from "react";
import { Button } from "../primitives/Button";
import { Input } from "../primitives/Input";
import type { MCPTransport, RegisterMCPRequest } from "../../api/types";
import {
  DEFAULT_TRANSPORT,
  buildConnect,
  connectFieldsComplete,
  errorField,
  type ConnectFields,
} from "../../lib/transport";
import { ConnectFieldsEditor } from "./ConnectFieldsEditor";

export type AddEndpointFormProps = {
  onCreate: (req: RegisterMCPRequest) => Promise<void>;
  onCancel: () => void;
};

/**
 * Rendered inside a centered Modal (see EndpointList) rather than the
 * mockup's inline sidebar panel — an explicit UX request, not a fidelity
 * choice.
 *
 * It takes a whole RegisterMCPRequest to its caller rather than
 * (name, command, args): since ADR-0017 there is no single shape that
 * describes every endpoint, and the form is the only place that knows which
 * transport was picked.
 */
export function AddEndpointForm({ onCreate, onCancel }: AddEndpointFormProps) {
  const [name, setName] = useState("");
  const [transport, setTransport] = useState<MCPTransport>(DEFAULT_TRANSPORT);
  const [fields, setFields] = useState<ConnectFields>({ command: "", args: "", url: "" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSubmit = name.trim() !== "" && connectFieldsComplete(transport, fields) && !submitting;

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await onCreate({
        name: name.trim(),
        transport,
        connect: buildConnect(transport, fields),
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add endpoint.");
    } finally {
      setSubmitting(false);
    }
  };

  // A rejected registration is usually about the url or the command, and the
  // server says which in its message; show it against that field and leave
  // the general slot for everything else (a duplicate name, a downstream
  // that simply didn't answer).
  const field = error ? errorField(error) : null;

  return (
    <div className="flex flex-col gap-3.5">
      <label className="flex flex-col gap-1.5">
        <span className="text-[11px] font-semibold text-subtle">Display name</span>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="postgres-ro"
          disabled={submitting}
          mono
        />
      </label>
      <ConnectFieldsEditor
        transport={transport}
        onTransportChange={setTransport}
        fields={fields}
        onFieldsChange={setFields}
        errorField={field}
        errorMessage={error}
        disabled={submitting}
        labelled
      />
      {error && field === null && <p className="text-[11px] text-danger">{error}</p>}
      <div className="flex gap-2 pt-1">
        <Button onClick={submit} disabled={!canSubmit} className="flex-1">
          Add endpoint
        </Button>
        <Button variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
