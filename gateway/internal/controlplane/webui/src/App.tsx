import { useEndpoints } from "./api/endpoints";
import { InstancesScreen } from "./components/instances/InstancesScreen";
import { UsersScreen } from "./components/users/UsersScreen";
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
        {screen === "instances" ? <InstancesScreen /> : <UsersScreen />}
      </AppShell>
    </ErrorBoundary>
  );
}
