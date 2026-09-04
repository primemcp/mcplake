import { useState } from "react";
import { Button } from "../primitives/Button";
import { Input } from "../primitives/Input";

export type AddEndpointFormProps = {
  onCreate: (name: string, url: string) => Promise<void>;
  onCancel: () => void;
};

export function AddEndpointForm({ onCreate, onCancel }: AddEndpointFormProps) {
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canSubmit = name.trim() !== "" && url.trim() !== "" && !submitting;

  const submit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      await onCreate(name.trim(), url.trim());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add endpoint.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="p-3 border border-accent rounded-[10px] bg-accent-soft flex flex-col gap-2">
      <div className="text-xs font-semibold">Add MCP endpoint</div>
      <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="display name" mono />
      <Input
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        placeholder="https://… or lambda arn"
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
