import { useEffect, useMemo, useState } from "react";
import { EmptyState } from "../primitives/EmptyState";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { useEndpoints } from "../../api/endpoints";
import { useAccessPolicies, useFilterPolicies } from "../../api/policies";
import { composeUsers } from "../../api/users";
import { clearStorage, readStorage, writeStorage } from "../../lib/storage";
import { UserDetail, type UserTab } from "./UserDetail";
import { UserList } from "./UserList";

type Selection = { kind: "user"; name: string } | { kind: "new" } | null;

// Only a real, saved user's selection is worth restoring -- a "new user"
// draft is unsaved by definition and was never meant to survive a reload
// (UserDetail's own draft state doesn't persist either).
const SELECTION_KEY = "mcplake:users:selection";
type StoredSelection = { name: string; tab: UserTab };
function isStoredSelection(v: unknown): v is StoredSelection {
  return (
    typeof v === "object" &&
    v !== null &&
    typeof (v as Record<string, unknown>).name === "string" &&
    ["match", "access", "path"].includes((v as Record<string, unknown>).tab as string)
  );
}

export type UsersScreenProps = {
  /** Switches the app to the MCP connections screen -- the Access tab's
   * "open in MCP connections" links use this. */
  onGoInstances: () => void;
};

/**
 * The screen-level wiring for issue #80: composes `User`s from the real
 * AccessPolicy + FilterPolicy lists (see api/users.ts), and owns the one
 * piece of state neither UserList nor UserDetail can own themselves --
 * which user (or "drafting a brand new one") is selected. `endpoints`
 * comes from the same `useEndpoints` MCP connections screen uses, since
 * the Access tab's Step 1 grants against exactly that same set.
 */
export function UsersScreen({ onGoInstances }: UsersScreenProps) {
  const { endpoints } = useEndpoints();
  const accessPolicies = useAccessPolicies();
  const filterPolicies = useFilterPolicies();
  const [selection, setSelection] = useState<Selection>(() => {
    const stored = readStorage(SELECTION_KEY, isStoredSelection);
    return stored ? { kind: "user", name: stored.name } : null;
  });
  const [tab, setTab] = useState<UserTab>(() => readStorage(SELECTION_KEY, isStoredSelection)?.tab ?? "match");

  const users = useMemo(
    () => composeUsers(accessPolicies.accessPolicies, filterPolicies.filterPolicies),
    [accessPolicies.accessPolicies, filterPolicies.filterPolicies],
  );

  const loading = accessPolicies.loading || filterPolicies.loading;
  const error = accessPolicies.error ?? filterPolicies.error;
  const retry = () => {
    accessPolicies.retry();
    filterPolicies.retry();
  };

  // A selected user can vanish out from under the selection (deleted from
  // another tab, or -- more commonly here -- this very screen's own
  // delete flow already clears it, but a retry/refresh after an external
  // change should too). Gated on `loading` so a selection just restored
  // from localStorage isn't discarded on the very first render, before
  // `users` has actually loaded and could confirm it still exists.
  useEffect(() => {
    if (loading) return;
    if (selection?.kind === "user" && !users.some((u) => u.name === selection.name)) {
      setSelection(null);
      clearStorage(SELECTION_KEY);
    }
  }, [users, selection, loading]);

  const selectUser = (name: string) => {
    setSelection({ kind: "user", name });
    setTab("match");
    writeStorage(SELECTION_KEY, { name, tab: "match" });
  };

  const changeTab = (next: UserTab) => {
    setTab(next);
    if (selection?.kind === "user") writeStorage(SELECTION_KEY, { name: selection.name, tab: next });
  };

  const selectedUser = selection?.kind === "user" ? (users.find((u) => u.name === selection.name) ?? null) : null;

  return (
    <div className="flex-1 min-h-0 flex flex-col">
      <div className="px-6 pt-4.5 pb-3.5 bg-surface border-b border-border flex items-end justify-between gap-5">
        <div className="flex flex-col gap-0.5">
          <h1 className="m-0 text-[19px] font-semibold tracking-tight">Users & access</h1>
          <p className="m-0 text-[12.5px] text-subtle">Who a token resolves to, and what they can reach.</p>
        </div>
        <div className="text-xs text-subtle">
          {users.length} {users.length === 1 ? "user" : "users"}
        </div>
      </div>

      <div className="flex-1 min-h-0 flex">
        <UserList
          users={users}
          loading={loading}
          error={error}
          onRetry={retry}
          selectedName={selection?.kind === "user" ? selection.name : null}
          onSelect={selectUser}
          onAddNew={() => setSelection({ kind: "new" })}
        />

        {/* min-h-0 is load-bearing here: without it, a flex row's cross-axis
            item defaults to min-height:auto (its content's natural height),
            so UserDetail could stretch this wrapper taller than the
            available row height instead of ever being bounded by it. That
            starves AccessTab's own internal scroll regions (Step 1/Step 2)
            of a fixed height to scroll *within*, so this outer div's own
            overflow-y-auto ends up swallowing everything as one long
            scroll instead -- exactly the "MCP endpoints doesn't scroll"
            symptom this fixes. */}
        <div className="flex-1 min-w-0 min-h-0 overflow-y-auto">
          {error ? (
            <div className="p-5">
              <ErrorNotice onRetry={retry} />
            </div>
          ) : loading ? null : selection && (selection.kind === "new" || selectedUser) ? (
            <UserDetail
              // Remounts the whole draft on selection change -- same
              // reasoning as ResponseFilterGroup's key on the MCP
              // connections screen: without it, an in-progress edit to
              // one user's Token match or Access tab would otherwise
              // leak into whichever user (or "new user") is selected
              // next, since React would just re-render with new props
              // rather than resetting local draft state.
              key={selection.kind === "user" ? selection.name : "__new__"}
              user={selection.kind === "user" ? selectedUser : null}
              endpoints={endpoints}
              allFilters={filterPolicies.filterPolicies}
              allAccessPolicies={accessPolicies.accessPolicies}
              onGoInstances={onGoInstances}
              onCreateAccessPolicy={accessPolicies.create}
              onUpdateAccessPolicy={accessPolicies.update}
              onDeleteAccessPolicy={accessPolicies.remove}
              onCreateFilter={filterPolicies.create}
              onUpdateFilter={filterPolicies.update}
              onDeleteFilter={filterPolicies.remove}
              initialTab={selection.kind === "user" ? tab : "match"}
              onTabChange={changeTab}
              onSaved={(name) => {
                // Preserve whichever tab was active -- e.g. saving from
                // the Access tab shouldn't bounce back to Token match.
                setSelection({ kind: "user", name });
                writeStorage(SELECTION_KEY, { name, tab });
              }}
              onDeleted={() => {
                setSelection(null);
                clearStorage(SELECTION_KEY);
              }}
            />
          ) : (
            <div className="flex items-center justify-center h-full p-6">
              <EmptyState title="No user selected" subtitle="Pick one from the left, or add a new one." />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
