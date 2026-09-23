import type { ConnectConfig, MCPRegistration, MCPTransport } from "../api/types";

/**
 * The transports the gateway can actually reach an MCP over, in the order
 * the picker offers them. Mirrors mcp.SupportedTransports() (mcp/transport.go);
 * ADR-0017 notes these strings are a wire format -- they are persisted and
 * accepted verbatim -- so this list is not free to be re-spelled here.
 */
export const TRANSPORTS: readonly MCPTransport[] = ["stdio", "http", "sse"] as const;

export const DEFAULT_TRANSPORT: MCPTransport = "stdio";

/** What each transport is, in one line, for the picker's tooltip. */
export const TRANSPORT_DESCRIPTIONS: Record<MCPTransport, string> = {
  stdio: "The gateway runs the server as a subprocess of itself",
  http: "MCP Streamable HTTP — a server running somewhere else",
  sse: "The older HTTP+SSE transport, for servers that only speak it",
};

/**
 * Whether a transport is reached over the network (and so is configured by
 * URL) rather than by starting a process (configured by command).
 */
export function isRemoteTransport(transport: string): boolean {
  return transport === "http" || transport === "sse";
}

/** Narrows an arbitrary stored transport string to one the picker can show. */
export function asTransport(value: string): MCPTransport {
  return (TRANSPORTS as readonly string[]).includes(value)
    ? (value as MCPTransport)
    : DEFAULT_TRANSPORT;
}

export type ConnectFields = {
  command: string;
  args: string;
  url: string;
};

/**
 * Builds the connect config for `transport`, carrying *only* the fields that
 * transport uses.
 *
 * This is not tidiness. The backend rejects a `url` on a stdio entry and a
 * `command` on an http/sse one (config.validateMCP, and mcp.NewClient for the
 * admin API path) precisely because silently ignoring the wrong field means
 * connecting to something other than what the operator wrote down. So a form
 * that keeps both fields in state -- as ours does, so switching transport back
 * and forth doesn't lose what you typed -- has to drop the irrelevant one at
 * the boundary, or a correctly-filled form turns into a 400.
 */
export function buildConnect(transport: MCPTransport, fields: ConnectFields): ConnectConfig {
  if (isRemoteTransport(transport)) {
    return { url: fields.url.trim() };
  }
  const args = fields.args.trim();
  return {
    command: fields.command.trim(),
    arguments: args === "" ? undefined : args.split(/\s+/),
  };
}

/** Whether the fields required by `transport` are filled in. */
export function connectFieldsComplete(transport: MCPTransport, fields: ConnectFields): boolean {
  return isRemoteTransport(transport) ? fields.url.trim() !== "" : fields.command.trim() !== "";
}

/**
 * How an endpoint's target reads in a header or list row: the command line
 * for stdio, the URL for anything remote.
 */
export function endpointTarget(endpoint: MCPRegistration): string {
  if (isRemoteTransport(endpoint.transport)) return endpoint.connect.url ?? "";
  const args = endpoint.connect.arguments ?? [];
  return [endpoint.connect.command ?? "", ...args].join(" ").trim();
}

/**
 * Which form field a failed registration is about, so the server's complaint
 * can be shown against that field rather than as a banner the operator has to
 * map back onto the form themselves.
 *
 * Matching on message text is not lovely, but the admin API returns one
 * `{error, message}` shape with no field information, and inventing a
 * machine-readable field code on the server for the UI's benefit is a bigger
 * change than this earns. The strings matched here are the ones mcp.NewClient
 * produces, and getting this wrong degrades to showing the message in the
 * general error slot -- the operator still sees it either way.
 */
export function errorField(message: string): "url" | "command" | null {
  if (/Config\.URL|must use https|absolute http\(s\) URL|must include a host/i.test(message)) {
    return "url";
  }
  if (/Config\.Command/i.test(message)) return "command";
  return null;
}
