import { useState } from "react";
import { Button } from "../primitives/Button";
import { Input } from "../primitives/Input";
import { Tabs } from "../primitives/Tabs";
import type { AccessPolicyRequest, ClaimRule, FilterPolicyRequest, Grant, MCPRegistration } from "../../api/types";
import { planUserSave, type User, type UserFieldsByEndpoint } from "../../api/users";
import { AccessTab } from "./AccessTab";
import { TokenMatchTab } from "./TokenMatchTab";

export type UserDetailProps = {
  /** null means a brand new, not-yet-saved user is being drafted. Pass a
   * `key` from the parent (the user's name, or a fixed sentinel while
   * creating) so switching *which* user is selected remounts this and
   * resets the draft -- same pattern InstancesScreen uses for
   * ResponseFilterGroup. */
  user: User | null;
  endpoints: MCPRegistration[];
  onCreateAccessPolicy: (req: AccessPolicyRequest) => Promise<void>;
  onUpdateAccessPolicy: (name: string, req: AccessPolicyRequest) => Promise<void>;
  onDeleteAccessPolicy: (name: string) => Promise<void>;
  onCreateFilter: (req: FilterPolicyRequest) => Promise<void>;
  onUpdateFilter: (name: string, req: FilterPolicyRequest) => Promise<void>;
  onDeleteFilter: (name: string) => Promise<void>;
  onSaved: (name: string) => void;
  onDeleted: () => void;
};

function fieldsFromUser(user: User | null): UserFieldsByEndpoint {
  const result: UserFieldsByEndpoint = {};
  for (const f of user?.filters ?? []) {
    (result[f.mcp] ??= {})[f.tool] = f.drop_fields;
  }
  return result;
}

type Draft = { name: string; match: ClaimRule[]; grants: Grant[]; fields: UserFieldsByEndpoint };

function draftOf(user: User | null): Draft {
  return { name: user?.name ?? "", match: user?.match ?? [], grants: user?.grants ?? [], fields: fieldsFromUser(user) };
}

const TABS = [
  { id: "match", label: "Token match" },
  { id: "access", label: "Access" },
];

/**
 * Tab container + the sticky Save/Discard model the mockup's detail panel
 * uses: every edit (across both tabs) is local draft state until Save,
 * which reconciles the whole thing in one go via planUserSave (see
 * api/users.ts) -- one AccessPolicy upsert plus whatever FilterPolicy
 * create/update/delete calls the diff against what's currently persisted
 * actually needs. No per-tab auto-save (unlike ResponseFilterGroup on the
 * MCP connections screen, which saves each filter the moment its own form
 * is submitted) -- this panel's Save commits everything together, matching
 * the mockup's explicit footer.
 *
 * A saved FilterPolicy always carries the *user's* current match rules,
 * never its own -- TokenMatchTab's doc comment covers why (kept in sync on
 * every save, so a user's response filters always apply under the same
 * conditions their access grant does).
 *
 * Request path is issue #81's tab, not built here -- #80's own scope says
 * so explicitly ("this task covers the first two [tabs]").
 */
export function UserDetail({
  user,
  endpoints,
  onCreateAccessPolicy,
  onUpdateAccessPolicy,
  onDeleteAccessPolicy,
  onCreateFilter,
  onUpdateFilter,
  onDeleteFilter,
  onSaved,
  onDeleted,
}: UserDetailProps) {
  const isNew = user === null;
  const [initial] = useState(() => draftOf(user));
  const [name, setName] = useState(initial.name);
  const [match, setMatch] = useState(initial.match);
  const [grants, setGrants] = useState(initial.grants);
  const [fields, setFields] = useState(initial.fields);
  const [tab, setTab] = useState("match");
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const dirty =
    name !== initial.name ||
    JSON.stringify(match) !== JSON.stringify(initial.match) ||
    JSON.stringify(grants) !== JSON.stringify(initial.grants) ||
    JSON.stringify(fields) !== JSON.stringify(initial.fields);

  const canSave = name.trim() !== "" && match.length > 0 && (isNew || dirty);

  const discard = () => {
    setName(initial.name);
    setMatch(initial.match);
    setGrants(initial.grants);
    setFields(initial.fields);
    setError(null);
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const plan = planUserSave(name.trim(), user, { match, grants }, fields);
      if (isNew) {
        await onCreateAccessPolicy(plan.accessPolicy);
      } else {
        await onUpdateAccessPolicy(user.name, plan.accessPolicy);
      }
      await Promise.all([
        ...plan.filtersToCreate.map((f) =>
          onCreateFilter({ name: f.name, match, mcp: f.mcp, tool: f.tool, drop_fields: f.dropFields }),
        ),
        ...plan.filtersToUpdate.map((f) =>
          onUpdateFilter(f.name, { name: f.name, match, mcp: f.mcp, tool: f.tool, drop_fields: f.dropFields }),
        ),
        ...plan.filtersToDelete.map((n) => onDeleteFilter(n)),
      ]);
      onSaved(name.trim());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save.");
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (isNew) return;
    if (!window.confirm(`Delete ${user.name}? This removes their access grant and all response filters.`)) return;
    setDeleting(true);
    try {
      await Promise.all([onDeleteAccessPolicy(user.name), ...user.filters.map((f) => onDeleteFilter(f.name))]);
      onDeleted();
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div className="flex flex-col h-full">
      <div className="flex-1 min-h-0 overflow-y-auto p-5 flex flex-col gap-4">
        <div className="flex items-center gap-2.5">
          {isNew ? (
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="user name" className="flex-1" />
          ) : (
            <h2 className="m-0 text-base font-semibold tracking-tight">{name}</h2>
          )}
          {!isNew && (
            <Button variant="danger" onClick={remove} disabled={deleting} className="ml-auto">
              Delete user
            </Button>
          )}
        </div>

        <Tabs tabs={TABS} activeId={tab} onChange={setTab} />

        {tab === "match" ? (
          <TokenMatchTab match={match} onChange={setMatch} />
        ) : (
          <AccessTab
            endpoints={endpoints}
            grants={grants}
            onGrantsChange={setGrants}
            fieldsByEndpoint={fields}
            onFieldsByEndpointChange={setFields}
          />
        )}

        {error && <p className="text-[10.5px] text-danger">{error}</p>}
      </div>

      <div className="shrink-0 p-3.5 border-t border-border bg-surface flex items-center gap-2">
        <Button onClick={save} disabled={!canSave || saving}>
          {isNew ? "Save user" : "Save changes"}
        </Button>
        {dirty && (
          <Button variant="secondary" onClick={discard}>
            Discard
          </Button>
        )}
      </div>
    </div>
  );
}
