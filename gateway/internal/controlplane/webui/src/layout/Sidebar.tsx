import { SignedInAs } from "../auth/SignedInAs";
import type { Screen } from "../state/useNav";

export type SidebarProps = {
  screen: Screen;
  onGoInstances: () => void;
  onGoUsers: () => void;
  /** Aggregate MCP registration counts for the Schema Cache summary —
   * cachedCount is how many endpoints have discovered tool schemas,
   * connectedCount is how many report status "active". */
  cachedCount: number;
  connectedCount: number;
  totalCount: number;
};

// Matches the mockup's own `nav(active)` helper exactly:
// `active ? [C.accentSoft, C.accent] : ['transparent', C.ink2]` — a soft
// blue pill with accent-blue text when active, not a filled dark pill
// (that was the bug: this file previously used bg-ink/text-white here).
const navItemClass = (active: boolean) =>
  `flex items-center gap-2.5 text-left px-2.5 py-2 rounded-lg border-0 cursor-pointer text-[13px] font-medium ${
    active ? "bg-select-soft text-accent" : "bg-transparent text-body"
  }`;

export function Sidebar({
  screen,
  onGoInstances,
  onGoUsers,
  cachedCount,
  connectedCount,
  totalCount,
}: SidebarProps) {
  const cacheBarPct = totalCount === 0 ? 0 : Math.round((connectedCount / totalCount) * 100);

  return (
    <aside className="w-[232px] shrink-0 bg-surface border-r border-border flex flex-col p-3.5 gap-0.5">
      <div className="flex items-center gap-2.5 px-1.5 pb-4">
        <div className="w-7 h-7 rounded-lg bg-ink grid place-items-center text-white font-mono text-[13px] font-medium">
          g
        </div>
        <div className="flex flex-col leading-tight">
          <div className="text-[13.5px] font-semibold">Gateway</div>
          <div className="text-[11px] text-muted">MCP · Phase 1</div>
        </div>
      </div>

      <nav className="flex flex-col gap-0.5">
        <button type="button" onClick={onGoInstances} className={navItemClass(screen === "instances")}>
          <span className="w-1.5 h-1.5 rounded-full bg-current opacity-55" />
          MCP connections
        </button>
        <button type="button" onClick={onGoUsers} className={navItemClass(screen === "users")}>
          <span className="w-1.5 h-1.5 rounded-full bg-current opacity-55" />
          Users &amp; access
        </button>
      </nav>

      <div className="mt-5 px-2.5 text-[10.5px] font-semibold tracking-wide uppercase text-muted">
        Schema Cache
      </div>
      <div className="mt-2 mx-1 p-2.5 border border-border rounded-[10px] flex flex-col gap-1.5">
        <div className="flex justify-between text-xs text-body">
          <span>Cached schemas</span>
          <span className="font-mono">{cachedCount}</span>
        </div>
        <div className="flex justify-between text-xs text-body">
          <span>Connected</span>
          <span className="font-mono">
            {connectedCount}/{totalCount}
          </span>
        </div>
        <div className="h-1 rounded-full bg-border-soft overflow-hidden">
          <div className="h-full bg-accent" style={{ width: `${cacheBarPct}%` }} />
        </div>
      </div>

      <div className="flex-1" />
      {/* Renders nothing when admin auth is off (see SignedInAs). */}
      <div className="mb-1.5">
        <SignedInAs />
      </div>
      <div className="px-2.5 py-2.5 rounded-[10px] bg-bg flex flex-col gap-1">
        <div className="flex items-center gap-1.5 text-xs font-medium">
          <span className="w-1.5 h-1.5 rounded-full bg-success" />
          Gateway healthy
        </div>
        <div className="text-[11px] text-muted font-mono">self-hosted · zero-egress</div>
      </div>
    </aside>
  );
}
