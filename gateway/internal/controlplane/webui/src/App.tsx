import { useEndpoints } from "./api/endpoints";
import { EmptyState } from "./components/primitives/EmptyState";
import { InstancesScreen } from "./components/instances/InstancesScreen";
import { ErrorBoundary } from "./ErrorBoundary";
import { AppShell } from "./layout/AppShell";
import { useNav } from "./state/useNav";

export function App() {
  const { screen, goInstances, goUsers } = useNav();
  const { endpoints } = useEndpoints();

  const totalCount = endpoints.length;
  const connectedCount = endpoints.filter((e) => e.status === "active").length;
  const cachedCount = endpoints.filter((e) => Object.keys(e.tools ?? {}).length > 0).length;

  return (
    <ErrorBoundary>
      <AppShell
        screen={screen}
        onGoInstances={goInstances}
        onGoUsers={goUsers}
        cachedCount={cachedCount}
        connectedCount={connectedCount}
        totalCount={totalCount}
      >
        {screen === "instances" ? (
          <InstancesScreen />
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
