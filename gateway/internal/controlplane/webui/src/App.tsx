import { EmptyState } from "./components/primitives/EmptyState";
import { ErrorBoundary } from "./ErrorBoundary";
import { AppShell } from "./layout/AppShell";
import { useNav } from "./state/useNav";

export function App() {
  const { screen, goInstances, goUsers } = useNav();

  return (
    <ErrorBoundary>
      <AppShell
        screen={screen}
        onGoInstances={goInstances}
        onGoUsers={goUsers}
        cachedCount={0}
        connectedCount={0}
        totalCount={0}
      >
        {screen === "instances" ? (
          <div className="flex-1 flex items-center justify-center p-6">
            <EmptyState
              title="MCP connections screen"
              subtitle="Lands in #79 — endpoint list, edit, and response filters."
            />
          </div>
        ) : (
          <div className="flex-1 flex items-center justify-center p-6">
            <EmptyState
              title="Users & access screen"
              subtitle="Lands in #80/#81 — token match, access grants, request path."
            />
          </div>
        )}
      </AppShell>
    </ErrorBoundary>
  );
}
