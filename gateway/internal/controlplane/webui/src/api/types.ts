// Mirrors the Go DTOs in gateway/internal/controlplane/{mcps,access_policies,filter_policies}.go
// exactly (field names, json tags, optionality) — hand-written rather than
// generated, since the surface is small and stable (see the Admin UI design
// doc, "Why this shape").

export type ErrorResponse = { error: string; message: string };

export type ClaimRule = { path: string; pattern: string };

export type Grant = { mcp: string; tools: string[] };

export type AccessPolicy = { name: string; match: ClaimRule[]; grants: Grant[] };
export type AccessPolicyRequest = { name: string; match: ClaimRule[]; grants: Grant[] };

export type FilterPolicy = {
  name: string;
  match: ClaimRule[];
  mcp: string;
  tool: string;
  drop_fields: string[];
};
export type FilterPolicyRequest = FilterPolicy;

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
  tools?: Record<string, ToolSchema>;
};

export type RegisterMCPRequest = {
  name: string;
  transport?: string;
  connect: ConnectConfig;
};
