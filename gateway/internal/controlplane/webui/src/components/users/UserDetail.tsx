import { useState } from "react";
import { Button } from "../primitives/Button";
import { Card } from "../primitives/Card";
import { Input } from "../primitives/Input";
import { Tabs } from "../primitives/Tabs";
import type {
  AccessPolicy,
  AccessPolicyRequest,
  ClaimRule,
  FilterPolicy,
  FilterPolicyRequest,
  Grant,
  MCPRegistration,
} from "../../api/types";
import type { User } from "../../api/users";
import { AccessTab } from "./AccessTab";
import { RequestPathTab } from "./RequestPathTab";
import { TokenMatchTab } from "./TokenMatchTab";

export type UserTab = "match" | "access" | "path";

export type UserDetailProps = {
  /** null means a brand new, not-yet-saved user is being drafted. Pass a
   * `key` from the parent (the user's name, or a fixed sentinel while
   * creating) so switching *which* user is selected remounts this and
   * resets the draft -- same pattern InstancesScreen uses for
   * ResponseFilterGroup. */
  user: User | null;
  endpoints: MCPRegistration[];
  /** Every real FilterPolicy in the system, not just this user's -- the
   * Access tab groups these under `<user>::...` (see AccessTab/ADR-0010). */
  allFilters: FilterPolicy[];
  /** Every AccessPolicy in the system -- the Request path tab simulates
   * Engine.Authorize, which is a union over all of them, not just this
   * user's (see RequestPathTab). */
  allAccessPolicies: AccessPolicy[];
  onGoInstances: () => void;
  onCreateAccessPolicy: (req: AccessPolicyRequest) => Promise<void>;
  onUpdateAccessPolicy: (name: string, req: AccessPolicyRequest) => Promise<void>;
  onDeleteAccessPolicy: (name: string) => Promise<void>;
  onCreateFilter: (req: FilterPolicyRequest) => Promise<void>;
  onUpdateFilter: (name: string, req: FilterPolicyRequest) => Promise<void>;
  onDeleteFilter: (name: string) => Promise<void>;
  onSaved: (name: string) => void;
  onDeleted: () => void;
  /** Seeds which tab opens first (default "match"). The parent uses this
   * to restore the tab a reload would otherwise reset -- see onTabChange. */
  initialTab?: UserTab;
  /** Fired whenever the active tab changes, so the parent can persist it
   * (localStorage) for the next page load. Reload itself always remounts
   * this component fresh, so there's no internal state to restore from --
   * only the parent survives a reload to seed `initialTab`. */
  onTabChange?: (tab: UserTab) => void;
};

type Draft = { name: string; match: ClaimRule[]; grants: Grant[] };

function draftOf(user: User | null): Draft {
  return { name: user?.name ?? "", match: user?.match ?? [], grants: user?.grants ?? [] };
}

/**
 * Tab container + the sticky Save/Discard model the mockup's detail panel
 * uses for Token match and Access's *grants*: both are local draft state
 * until Save, which upserts the AccessPolicy in one call. Response
 * filters (Access tab, step 2) are the one exception -- they save
 * immediately as they're created/edited/deleted (see AccessTab's own doc
 * comment for why: a FilterPolicy is a fully independent backend record,
 * not part of any atomic transaction with the AccessPolicy).
 *
 * Because filters save independently, editing Token match here and then
 * clicking Save wouldn't otherwise reach a filter that was created
 * earlier under the old match -- so `save()` also re-PUTs every existing
 * filter whose match has drifted from the current draft, keeping them in
 * sync the way TokenMatchTab's own doc comment describes.
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
 * Request path (#81) is the third tab, shown only for a saved user: it
 * simulates against persisted policy (see RequestPathTab), so a brand
 * new draft has nothing for it to run against yet. It reads this panel's
 * `dirty` flag to warn when Token match / Access edits aren't saved. The
 * mockup's actual "new user" flow is a compact 3-field inline form in the
 * left list (name, claim path, regex) rather than a full draft through
 * this panel -- reusing this panel's tabs for `isNew` instead is a
 * simplification, not yet re-verified against that flow.
 */
export function UserDetail({
  user,
  endpoints,
  allFilters,
  allAccessPolicies,
  onGoInstances,
  onCreateAccessPolicy,
  onUpdateAccessPolicy,
  onDeleteAccessPolicy,
  onCreateFilter,
  onUpdateFilter,
  onDeleteFilter,
  onSaved,
  onDeleted,
  initialTab = "match",
  onTabChange,
}: UserDetailProps) {
  const isNew = user === null;
  const [initial, setInitial] = useState(() => draftOf(user));
  const [name, setName] = useState(initial.name);
  const [match, setMatch] = useState(initial.match);
  const [grants, setGrants] = useState(initial.grants);
  const [tab, setTab] = useState<UserTab>(initialTab);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const dirty =
    name !== initial.name ||
    JSON.stringify(match) !== JSON.stringify(initial.match) ||
    JSON.stringify(grants) !== JSON.stringify(initial.grants);

  // Requiring at least one condition guards against the *dangerous*
  // direction, not a useless one: router.ClaimMatcher with zero rules
  // matches every token (see router/claimrule.go), so an empty match
  // would grant this user's access to anyone, not resolve to no one.
  const canSave = name.trim() !== "" && match.length > 0 && (isNew || dirty);

  const tabs = [
    { id: "match", label: "Token match" },
    { id: "access", label: `Access ${grants.length}` },
    ...(isNew ? [] : [{ id: "path", label: "Request path" }]),
  ];
  const tabHint =
    tab === "access"
      ? "one grant per endpoint · a filter belongs to exactly one endpoint"
      : tab === "path"
        ? "what the gateway does with one request from this user"
        : "JSONPath + regex · all conditions must pass (AND)";
  const changeTab = (next: UserTab) => {
    setTab(next);
    onTabChange?.(next);
  };

  const discard = () => {
    setName(initial.name);
    setMatch(initial.match);
    setGrants(initial.grants);
    setError(null);
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const accessPolicy: AccessPolicyRequest = { name: name.trim(), match, grants };
      if (isNew) {
        await onCreateAccessPolicy(accessPolicy);
      } else {
        await onUpdateAccessPolicy(user.name, accessPolicy);
        // Filters save independently as they're edited in the Access
        // tab (see its own doc comment) -- re-sync any that were created
        // or last updated under an older match, so a Token-match edit
        // here doesn't leave them silently out of date.
        await Promise.all(
          user.filters
            .filter((f) => JSON.stringify(f.match) !== JSON.stringify(match))
            .map((f) => onUpdateFilter(f.name, { ...f, match })),
        );
      }
      // Saving an *existing* user keeps the same selection.name, so the
      // parent's `key` doesn't change and this component never remounts
      // -- without re-baselining here, `dirty`/`canSave` would keep
      // comparing against the pre-save draft and the button would stay
      // active forever after a successful save.
      setInitial({ name: name.trim(), match, grants });
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

  // The Access tab's Step 1 / Step 2 columns scroll internally (see
  // AccessTab) -- matches the mockup, where the whole detail card grows
  // to fill the panel (flex: 1 1 auto) only on that tab, instead of the
  // page itself scrolling. Token match stays a normal, page-scrolling
  // panel since it never needs its own internal scroll region.
  const isAccessTab = tab === "access";
  // A persisted "path" tab can only have come from a saved user, but the
  // parent seeds "match" for a new draft anyway -- this is belt and braces.
  const isPathTab = tab === "path" && user !== null;

  return (
    <div className="flex flex-col h-full">
      <div
        className={`flex-1 min-h-0 flex flex-col gap-4 p-5 ${isAccessTab ? "overflow-hidden" : "overflow-y-auto"}`}
      >
        {isNew && (
          <Card className="p-3 shrink-0">
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="user name" />
          </Card>
        )}

        <Card className="overflow-hidden shrink-0">
          <div className="p-[11px_16px] border-b border-border-soft flex flex-wrap items-center gap-[8px_12px]">
            <Tabs
              tabs={tabs}
              activeId={tab}
              onChange={(id) => changeTab(id as UserTab)}
            />
            <div className="text-[11.5px] text-muted min-w-0">{tabHint}</div>
            <span className="flex-1" />
            {!isNew && (
              <Button variant="danger" onClick={remove} disabled={deleting}>
                Delete user
              </Button>
            )}
          </div>
          {!isAccessTab && !isPathTab && (
            <div className="p-4">
              <TokenMatchTab match={match} onChange={setMatch} />
            </div>
          )}
          {isPathTab && (
            <RequestPathTab
              user={user}
              endpoints={endpoints}
              allAccessPolicies={allAccessPolicies}
              allFilters={allFilters}
              draftDirty={dirty}
              onGoAccess={() => changeTab("access")}
              onGoInstances={onGoInstances}
            />
          )}
        </Card>

        {isAccessTab && (
          <Card className="flex-1 min-h-0 overflow-hidden p-4">
            <AccessTab
              endpoints={endpoints}
              grants={grants}
              onGrantsChange={setGrants}
              allFilters={allFilters}
              onGoInstances={onGoInstances}
              userName={isNew ? null : user.name}
              userMatch={match}
              onCreateFilter={onCreateFilter}
              onUpdateFilter={onUpdateFilter}
              onDeleteFilter={onDeleteFilter}
            />
          </Card>
        )}

        {error && <p className="text-[10.5px] text-danger shrink-0">{error}</p>}
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
