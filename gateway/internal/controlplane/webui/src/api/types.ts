// Mirrors the Go DTOs in gateway/internal/controlplane/{mcps,access_policies,filter_policies}.go
// exactly (field names, json tags, optionality) — hand-written rather than
// generated, since the surface is small and stable (see the Admin UI design
// doc, "Why this shape").

export type ErrorResponse = { error: string; message: string };

export type ClaimRule = { path: string; pattern: string };

export type Grant = { mcp: string; tools: string[] };

export type AccessPolicy = { name: string; match: ClaimRule[]; grants: Grant[]; enabled: boolean };
// `enabled` is optional on write: the real API defaults a missing value to
// true (see enabledOrTrue in the Go handler) -- omit it to just mean "on".
export type AccessPolicyRequest = { name: string; match: ClaimRule[]; grants: Grant[]; enabled?: boolean };

export type FilterPolicy = {
  name: string;
  match: ClaimRule[];
  mcp: string;
  tool: string;
  drop_fields: string[];
  enabled: boolean;
};
export type FilterPolicyRequest = Omit<FilterPolicy, "enabled"> & { enabled?: boolean };

export type ConnectConfig = { command?: string; arguments?: string[]; url?: string };

export type ToolSchema = {
  name: string;
  input_schema?: unknown;
  output_schema?: unknown;
};

export type MCPTransport = "stdio" | "sse" | "http";

export type MCPRegistration = {
  name: string;
  transport: string;
  connect: ConnectConfig;
  status: string;
  enabled: boolean;
  tools?: Record<string, ToolSchema>;
};

export type RegisterMCPRequest = {
  name: string;
  transport?: string;
  connect: ConnectConfig;
  // Optional; omitting it defaults to true server-side (enabledOrTrue). The
  // "edit" flow (register() again with the same name) must pass the
  // endpoint's current value explicitly -- otherwise saving an unrelated
  // edit on a disabled endpoint would silently re-enable it.
  enabled?: boolean;
};

export type SetMCPEnabledRequest = { enabled: boolean };
