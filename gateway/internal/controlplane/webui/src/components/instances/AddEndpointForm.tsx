import { useState } from "react";
import { Button } from "../primitives/Button";
import { Input } from "../primitives/Input";

export type AddEndpointFormProps = {
  onCreate: (name: string, command: string, args: string[]) => Promise<void>;
  onCancel: () => void;
};

/**
 * Only stdio transport works against the real backend today (see
 * EndpointDetail's doc comment), so this collects a command rather than
 * the mockup's URL field — a URL-only add would register with an empty
 * Command and fail with "mcp: Config.Command is required" (found by
 * testing the running UI against the real API, not caught by unit tests
 * since those didn't exercise cache.Registry's real transport check).
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
    <div className="p-3 border border-accent rounded-[10px] bg-accent-soft flex flex-col gap-2">
      <div className="text-xs font-semibold">Add MCP endpoint</div>
      <p className="text-[10.5px] text-subtle">
        Only stdio (subprocess command) transport is implemented today.
      </p>
      <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="display name" mono />
      <Input
        value={command}
        onChange={(e) => setCommand(e.target.value)}
        placeholder="command, e.g. mcp-server-postgres"
        mono
      />
      <Input
        value={args}
        onChange={(e) => setArgs(e.target.value)}
        placeholder="arguments (space-separated, optional)"
        mono
      />
      {error && <p className="text-[10.5px] text-danger">{error}</p>}
      <div className="flex gap-1.5">
        <Button onClick={submit} disabled={!canSubmit} className="flex-1">
          Add
        </Button>
        <Button variant="secondary" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
