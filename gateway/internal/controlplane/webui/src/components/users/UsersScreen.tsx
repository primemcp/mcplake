import { useEffect, useMemo, useState } from "react";
import { EmptyState } from "../primitives/EmptyState";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { useEndpoints } from "../../api/endpoints";
import { useAccessPolicies, useFilterPolicies } from "../../api/policies";
import { composeUsers } from "../../api/users";
import { UserDetail } from "./UserDetail";
import { UserList } from "./UserList";

type Selection = { kind: "user"; name: string } | { kind: "new" } | null;

/**
 * The screen-level wiring for issue #80: composes `User`s from the real
 * AccessPolicy + FilterPolicy lists (see api/users.ts), and owns the one
 * piece of state neither UserList nor UserDetail can own themselves --
 * which user (or "drafting a brand new one") is selected. `endpoints`
 * comes from the same `useEndpoints` MCP connections screen uses, since
 * the Access tab's Step 1 grants against exactly that same set.
 */
export function UsersScreen() {
  const { endpoints } = useEndpoints();
  const accessPolicies = useAccessPolicies();
  const filterPolicies = useFilterPolicies();
  const [selection, setSelection] = useState<Selection>(null);

  const users = useMemo(
    () => composeUsers(accessPolicies.accessPolicies, filterPolicies.filterPolicies),
    [accessPolicies.accessPolicies, filterPolicies.filterPolicies],
  );

  // A selected user can vanish out from under the selection (deleted from
  // another tab, or -- more commonly here -- this very screen's own
  // delete flow already clears it, but a retry/refresh after an external
  // change should too).
  useEffect(() => {
    if (selection?.kind === "user" && !users.some((u) => u.name === selection.name)) {
      setSelection(null);
    }
  }, [users, selection]);

  const loading = accessPolicies.loading || filterPolicies.loading;
  const error = accessPolicies.error ?? filterPolicies.error;
  const retry = () => {
    accessPolicies.retry();
    filterPolicies.retry();
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
          onSelect={(name) => setSelection({ kind: "user", name })}
          onAddNew={() => setSelection({ kind: "new" })}
        />

        <div className="flex-1 min-w-0 overflow-y-auto">
          {error ? (
            <div className="p-5">
              <ErrorNotice onRetry={retry} />
            </div>
          ) : selection ? (
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
              onCreateAccessPolicy={accessPolicies.create}
              onUpdateAccessPolicy={accessPolicies.update}
              onDeleteAccessPolicy={accessPolicies.remove}
              onCreateFilter={filterPolicies.create}
              onUpdateFilter={filterPolicies.update}
              onDeleteFilter={filterPolicies.remove}
              onSaved={(name) => setSelection({ kind: "user", name })}
              onDeleted={() => setSelection(null)}
            />
          ) : (
            !loading && (
              <div className="flex items-center justify-center h-full p-6">
                <EmptyState title="No user selected" subtitle="Pick one from the left, or add a new one." />
              </div>
            )
          )}
        </div>
      </div>
    </div>
  );
}
