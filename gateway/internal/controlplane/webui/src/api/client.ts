import type {
  AccessPolicy,
  AccessPolicyRequest,
  ErrorResponse,
  FilterPolicy,
  FilterPolicyRequest,
  MCPRegistration,
  RegisterMCPRequest,
  SetMCPEnabledRequest,
} from "./types";

/**
 * Thrown for any non-2xx admin API response. `body` is the parsed
 * `{error, message}` payload every handler returns on failure, when the
 * response actually had one (a network-level failure or a non-JSON error
 * page has `body: null`).
 */
export class ApiError extends Error {
  readonly status: number;
  readonly body: ErrorResponse | null;

  constructor(status: number, body: ErrorResponse | null) {
    super(body?.message ?? `admin API request failed with status ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/admin${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const isJSON = res.headers.get("content-type")?.includes("application/json") ?? false;
  const body = isJSON ? await res.json() : null;

  if (!res.ok) {
    throw new ApiError(res.status, body as ErrorResponse | null);
  }
  return body as T;
}

const encode = (segment: string) => encodeURIComponent(segment);

export const api = {
  listMCPs: () => request<MCPRegistration[]>("/mcps"),
  registerMCP: (req: RegisterMCPRequest) =>
    request<MCPRegistration>("/mcps", { method: "POST", body: JSON.stringify(req) }),
  setMCPEnabled: (name: string, req: SetMCPEnabledRequest) =>
    request<MCPRegistration>(`/mcps/${encode(name)}`, { method: "PATCH", body: JSON.stringify(req) }),
  unregisterMCP: (name: string) => request<void>(`/mcps/${encode(name)}`, { method: "DELETE" }),

  listAccessPolicies: () => request<AccessPolicy[]>("/access-policies"),
  getAccessPolicy: (name: string) => request<AccessPolicy>(`/access-policies/${encode(name)}`),
  createAccessPolicy: (req: AccessPolicyRequest) =>
    request<AccessPolicy>("/access-policies", { method: "POST", body: JSON.stringify(req) }),
  updateAccessPolicy: (name: string, req: AccessPolicyRequest) =>
    request<AccessPolicy>(`/access-policies/${encode(name)}`, {
      method: "PUT",
      body: JSON.stringify(req),
    }),
  deleteAccessPolicy: (name: string) =>
    request<void>(`/access-policies/${encode(name)}`, { method: "DELETE" }),

  listFilterPolicies: () => request<FilterPolicy[]>("/filter-policies"),
  getFilterPolicy: (name: string) => request<FilterPolicy>(`/filter-policies/${encode(name)}`),
  createFilterPolicy: (req: FilterPolicyRequest) =>
    request<FilterPolicy>("/filter-policies", { method: "POST", body: JSON.stringify(req) }),
  updateFilterPolicy: (name: string, req: FilterPolicyRequest) =>
    request<FilterPolicy>(`/filter-policies/${encode(name)}`, {
      method: "PUT",
      body: JSON.stringify(req),
    }),
  deleteFilterPolicy: (name: string) =>
    request<void>(`/filter-policies/${encode(name)}`, { method: "DELETE" }),
};
