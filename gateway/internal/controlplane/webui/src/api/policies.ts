import { useCallback } from "react";
import { api } from "./client";
import type { AccessPolicyRequest, FilterPolicy, FilterPolicyRequest } from "./types";
import { useAsyncResource } from "./useAsyncResource";

export function useAccessPolicies() {
  const { data, loading, error, retry } = useAsyncResource(() => api.listAccessPolicies(), []);

  const create = useCallback(
    async (req: AccessPolicyRequest) => {
      await api.createAccessPolicy(req);
      await retry();
    },
    [retry],
  );
  const update = useCallback(
    async (name: string, req: AccessPolicyRequest) => {
      await api.updateAccessPolicy(name, req);
      await retry();
    },
    [retry],
  );
  const remove = useCallback(
    async (name: string) => {
      await api.deleteAccessPolicy(name);
      await retry();
    },
    [retry],
  );

  return { accessPolicies: data ?? [], loading, error, retry, create, update, remove };
}

export function useFilterPolicies() {
  const { data, loading, error, retry } = useAsyncResource(() => api.listFilterPolicies(), []);

  const create = useCallback(
    async (req: FilterPolicyRequest) => {
      await api.createFilterPolicy(req);
      await retry();
    },
    [retry],
  );
  const update = useCallback(
    async (name: string, req: FilterPolicyRequest) => {
      await api.updateFilterPolicy(name, req);
      await retry();
    },
    [retry],
  );
  const remove = useCallback(
    async (name: string) => {
      await api.deleteFilterPolicy(name);
      await retry();
    },
    [retry],
  );

  return { filterPolicies: data ?? [], loading, error, retry, create, update, remove };
}

export function filterPoliciesForMCP(policies: FilterPolicy[], mcp: string): FilterPolicy[] {
  return policies.filter((p) => p.mcp === mcp);
}
