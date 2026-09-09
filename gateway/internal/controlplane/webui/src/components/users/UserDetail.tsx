import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
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
 * Header layout matches the real mockup source exactly (fetched via
 * DesignSync, not guessed): one row -- tab bar (Access's label carries its
 * live grant count, e.g. "Access 2"), a short hint for whichever tab is
 * active, a spacer, then the panel's primary action on the right. The
 * user's name is NOT repeated as a heading here -- the mockup shows it
 * only once, in the list row on the left.
 *
 * Two disclosed deviations from that same mockup, both already decided
 * earlier: the mockup's right-side action is an Enabled/Disabled toggle
 * (soft-disable, `user.disabled`) -- there's no such flag on the real
 * `AccessPolicy`, and faking it via delete would destroy grants/filters,
 * so this ships a real "Delete user" button instead. And the mockup's
 * match-status pill (next to that toggle, "Token matches" / "Token does
 * not match") depends on a decoded-JWT preview this task doesn't build
 * (issue #81 owns that) -- omitted along with the preview it describes.
 * The plain tab hint text is static in the mockup (doesn't depend on that
 * preview), so both tabs' hints are reproduced verbatim.
 *
 * Request path is issue #81's own tab, not built here -- #80's own scope
 * says so explicitly ("this task covers the first two [tabs]"). The
 * mockup's actual "new user" flow is a compact 3-field inline form in the
 * left list (name, claim path, regex) rather than a full draft through
 * this panel -- reusing this panel's tabs for `isNew` instead is a
 * simplification, not yet re-verified against that flow.
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

  const tabs = [
    { id: "match", label: "Token match" },
    { id: "access", label: `Access ${grants.length}` },
  ];
  const tabHint =
    tab === "access"
      ? "one grant per endpoint · a filter belongs to exactly one endpoint"
      : "JSONPath + regex · all conditions must pass (AND)";

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
        {isNew && (
          <Card className="p-3">
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="user name" />
          </Card>
        )}

        <Card className="overflow-hidden">
          <div className="p-[11px_16px] border-b border-border-soft flex flex-wrap items-center gap-[8px_12px]">
            <Tabs tabs={tabs} activeId={tab} onChange={setTab} />
            <div className="text-[11.5px] text-muted min-w-0">{tabHint}</div>
            <span className="flex-1" />
            {!isNew && (
              <Button variant="danger" onClick={remove} disabled={deleting}>
                Delete user
              </Button>
            )}
          </div>
          <div className="p-4">
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
          </div>
        </Card>

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
