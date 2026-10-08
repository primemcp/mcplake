import { useCallback } from "react";
import { api } from "./client";
import type { MCPRegistration, RegisterMCPRequest } from "./types";
import { useAsyncResource } from "./useAsyncResource";

/**
 * The MCP connections screen's data: GET /admin/mcps is the only read this
 * needs (it already returns each endpoint's discovered tools/schemas — no
 * separate schema endpoint exists). register/unregister/setEnabled all
 * refresh the list afterward so the UI reflects the write without a manual
 * reload. There is no PUT for MCPs — "editing" is register() again with the
 * same name (an upsert). Enable/disable is the one real PATCH
 * (`PATCH /admin/mcps/:name`), a reversible soft-disable distinct from
 * unregister (DELETE), which drops the registration and its schema cache.
 */
export function useEndpoints() {
  const { data, loading, error, retry } = useAsyncResource(() => api.listMCPs(), []);

  const register = useCallback(
    async (req: RegisterMCPRequest) => {
      await api.registerMCP(req);
      await retry();
    },
    [retry],
  );

  const unregister = useCallback(
    async (name: string) => {
      await api.unregisterMCP(name);
      await retry();
    },
    [retry],
  );

  const setEnabled = useCallback(
    async (name: string, enabled: boolean) => {
      await api.setMCPEnabled(name, { enabled });
      await retry();
    },
    [retry],
  );

  return { endpoints: data ?? [], loading, error, retry, register, unregister, setEnabled };
}

export function findEndpoint(
  endpoints: MCPRegistration[],
  name: string,
): MCPRegistration | undefined {
  return endpoints.find((e) => e.name === name);
}
