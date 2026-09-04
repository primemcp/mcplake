import { useState, type ReactNode } from "react";
import { Button } from "../primitives/Button";
import { Input } from "../primitives/Input";

export type AddEndpointFormProps = {
  onCreate: (name: string, command: string, args: string[]) => Promise<void>;
  onCancel: () => void;
};

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-[11px] font-semibold text-subtle">{label}</span>
      {children}
    </label>
  );
}

/**
 * Rendered inside a centered Modal (see EndpointList) rather than the
 * mockup's inline sidebar panel — an explicit UX request, not a fidelity
 * choice. Only stdio transport works against the real backend today (see
 * EndpointDetail's doc comment), so this collects a command rather than
 * the mockup's URL field — a URL-only add would register with an empty
 * Command and fail with "mcp: Config.Command is required" (found by
 * testing the running UI against the real API).
 */
export function AddEndpointForm({ onCreate, onCancel }: AddEndpointFormProps) {
  const [name, setName] = useState("");
  const [command, setCommand] = useState("");
  const [args, setArgs] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSubmit = name.trim() !== "" && command.trim() !== "" && !submitting;

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const argList = args.trim() === "" ? [] : args.trim().split(/\s+/);
      await onCreate(name.trim(), command.trim(), argList);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add endpoint.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex flex-col gap-3.5">
      <p className="text-[11.5px] text-subtle leading-normal">
        Only stdio (subprocess command) transport is implemented today.
      </p>
      <Field label="Display name">
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="postgres-ro" mono />
      </Field>
      <Field label="Command">
        <Input
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          placeholder="mcp-server-postgres"
          mono
        />
      </Field>
      <Field label="Arguments (optional, space-separated)">
        <Input
          value={args}
          onChange={(e) => setArgs(e.target.value)}
          placeholder="--read-only --db mcp"
          mono
        />
      </Field>
      {error && <p className="text-[11px] text-danger">{error}</p>}
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
