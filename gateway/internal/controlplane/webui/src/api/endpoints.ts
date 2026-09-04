import { useCallback } from "react";
import { api } from "./client";
import type { MCPRegistration, RegisterMCPRequest } from "./types";
import { useAsyncResource } from "./useAsyncResource";

/**
 * The MCP connections screen's data: GET /admin/mcps is the only read this
 * needs (it already returns each endpoint's discovered tools/schemas — no
 * separate schema endpoint exists). register/unregister both refresh the
 * list afterward so the UI reflects the write without a manual reload.
 * There is no PATCH/PUT for MCPs — "editing" is register() again with the
 * same name (an upsert), matching POST /admin/mcps's real semantics.
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

  return { endpoints: data ?? [], loading, error, retry, register, unregister };
}

export function findEndpoint(
  endpoints: MCPRegistration[],
  name: string,
): MCPRegistration | undefined {
  return endpoints.find((e) => e.name === name);
}
