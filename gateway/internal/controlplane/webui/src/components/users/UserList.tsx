import { useMemo, useState } from "react";
import { ErrorNotice } from "../primitives/ErrorNotice";
import { SearchInput } from "../primitives/SearchInput";
import type { User } from "../../api/users";

export type UserListProps = {
  users: User[];
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  selectedName: string | null;
  onSelect: (name: string) => void;
  onAddNew: () => void;
};

/** Every condition, `path ~ pattern`, AND-ed -- matches TokenMatchTab's own
 * wording so the list and the detail panel never disagree about what "no
 * conditions" means for a user: `router.ClaimMatcher` with zero rules
 * matches *every* token (see router/claimrule.go), so this is a warning
 * about being wide open, not about being unreachable. */
function condSummary(u: User): string {
  if (u.match.length === 0) return "no conditions — matches every token";
  return u.match.map((r) => `${r.path} ~ ${r.pattern}`).join(" · ");
}

export function UserList({ users, loading, error, onRetry, selectedName, onSelect, onAddNew }: UserListProps) {
  const [query, setQuery] = useState("");

  const hits = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === "") return users;
    return users.filter((u) => u.name.toLowerCase().includes(q));
  }, [users, query]);

  return (
    <div className="w-[320px] shrink-0 border-r border-border bg-surface p-3 flex flex-col gap-2.5 overflow-hidden">
      <SearchInput value={query} onChange={setQuery} placeholder="Search users" />

      {error ? (
        <ErrorNotice onRetry={onRetry} />
      ) : loading ? (
        <p className="text-[11px] text-muted px-1">Loading…</p>
      ) : (
        <>
          <div className="text-[10.5px] text-muted px-1">
            {hits.length} of {users.length}
          </div>
          <div className="flex-1 min-h-[90px] overflow-y-auto flex flex-col gap-2">
            {hits.length === 0 && (
              <div className="p-3 border border-dashed border-line rounded-[10px] text-[11.5px] text-subtle">
                {users.length === 0 ? "No users yet." : "No user matches that."}
              </div>
            )}
            {hits.map((u) => {
              const selected = u.name === selectedName;
              // Scoped: has at least one condition, so it only resolves for
              // tokens that satisfy them. Unscoped (0 conditions) matches
              // every token -- a warning state, not a broken one.
              const scoped = u.match.length > 0;
              return (
                <button
                  key={u.name}
                  type="button"
                  onClick={() => onSelect(u.name)}
                  className={`text-left p-[11px_12px] border rounded-[11px] cursor-pointer flex flex-col gap-[3px] ${
                    selected ? "border-accent bg-select-soft" : "border-border bg-surface"
                  }`}
                >
                  <div className="flex items-center gap-[7px] min-w-0">
                    <span className={`w-[7px] h-[7px] rounded-full shrink-0 ${scoped ? "bg-success" : "bg-danger"}`} />
                    <span className={`text-[12.5px] font-semibold truncate ${selected ? "text-accent-hover" : "text-ink"}`}>
                      {u.name}
                    </span>
                  </div>
                  <div className={`text-[10.5px] font-mono truncate ${scoped ? (selected ? "text-subtle" : "text-muted") : "text-danger"}`}>
                    {condSummary(u)}
                  </div>
                  <div className={`text-[10.5px] ${selected ? "text-subtle" : "text-muted"}`}>
                    {u.grants.length} {u.grants.length === 1 ? "grant" : "grants"} · {u.filters.length}{" "}
                    {u.filters.length === 1 ? "filter" : "filters"}
                  </div>
                </button>
              );
            })}
          </div>
        </>
      )}

      <button
        type="button"
        onClick={onAddNew}
        className="flex items-center gap-2.5 p-3 border border-dashed border-line rounded-[10px] bg-surface cursor-pointer text-left"
      >
        <span className="w-4 h-4 rounded border border-dashed border-muted grid place-items-center text-[11px] text-muted">
          +
        </span>
        <span className="flex flex-col gap-0.5">
          <span className="text-[12.5px] font-semibold">Add user</span>
          <span className="text-[10.5px] text-muted">a condition is all it takes to resolve one</span>
        </span>
      </button>
    </div>
  );
}
