import type {
  AccessPolicy,
  AuthConfig,
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

/**
 * How this module reaches the auth layer without importing React state.
 * `AuthProvider` installs these on mount; with admin auth off (or before
 * the provider mounts) they are absent and requests go out unauthenticated,
 * exactly as they did before ADR-0014.
 */
export type AuthHooks = {
  /** Resolves the token to send, refreshing it first if it has expired. */
  getToken: () => Promise<string | null>;
  /**
   * Called for 401 and 403 so the auth layer can move the whole app to the
   * right screen. The two mean different things and must not be collapsed:
   * 401 is "sign in again", 403 is "signed in, but not an admin" (see
   * ADR-0014). The request still rejects with ApiError afterwards.
   */
  onAuthFailure: (status: number) => void;
};

let authHooks: AuthHooks | null = null;

export function configureAuth(hooks: AuthHooks | null): void {
  authHooks = hooks;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = authHooks ? await authHooks.getToken() : null;
  const res = await fetch(`/admin${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token === null ? {} : { Authorization: `Bearer ${token}` }),
      ...init?.headers,
    },
  });

  if (res.status === 401 || res.status === 403) {
    authHooks?.onAuthFailure(res.status);
  }

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
  /**
   * The one admin route that answers without a token (ADR-0014): the UI
   * calls it before it has any credentials, to find out whether it needs
   * them and where to get them.
   */
  authConfig: () => request<AuthConfig>("/auth/config"),

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
