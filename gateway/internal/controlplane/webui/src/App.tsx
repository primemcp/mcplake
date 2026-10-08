import { useEndpoints } from "./api/endpoints";
import { AuthProvider } from "./auth/AuthProvider";
import { InstancesScreen } from "./components/instances/InstancesScreen";
import { UsersScreen } from "./components/users/UsersScreen";
import { ErrorBoundary } from "./ErrorBoundary";
import { AppShell } from "./layout/AppShell";
import { useNav } from "./state/useNav";

/**
 * Everything that talks to the admin API. It lives under AuthProvider on
 * purpose: the hooks below fetch on mount, so they must not render until
 * the app is actually allowed to call `/admin/*` (see AuthProvider).
 */
function AdminApp() {
  const { screen, goInstances, goUsers } = useNav();
  const { endpoints } = useEndpoints();

  const totalCount = endpoints.length;
  const connectedCount = endpoints.filter((e) => e.status === "active").length;
  const cachedCount = endpoints.filter((e) => Object.keys(e.tools ?? {}).length > 0).length;

  return (
    <AppShell
      screen={screen}
      onGoInstances={goInstances}
      onGoUsers={goUsers}
      cachedCount={cachedCount}
      connectedCount={connectedCount}
      totalCount={totalCount}
    >
      {screen === "instances" ? <InstancesScreen /> : <UsersScreen onGoInstances={goInstances} />}
    </AppShell>
  );
}

export function App() {
  return (
    <ErrorBoundary>
      <AuthProvider>
        <AdminApp />
      </AuthProvider>
    </ErrorBoundary>
  );
}
