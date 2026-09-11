import type { AccessPolicy, ClaimRule, FilterPolicy, Grant, MCPRegistration } from "../api/types";
import { evaluateMatcher, type MatcherEvaluation } from "./claimMatch";

/**
 * Client-side simulation of the gateway's tool-call pipeline for one
 * request (#81): the same stages, in the same order and with the same
 * status codes, as `gateway/internal/toolcall.go`'s handler --
 *
 *   auth (token → claims)            401 unauthorized
 *   access (Engine.Authorize)        403 forbidden / 500 policy error
 *   endpoint (resolver)              403 mcp_disabled / 404 mcp_not_found
 *   tool (resolver)                  404 tool_not_found
 *   response filter (FieldsToRemove) 500 policy error
 *
 * -- evaluated against the policies as currently saved, not a live trace
 * of a real request (the design doc's Non-goals rule out a backend
 * preview endpoint, and observability is Phase 3). The one stage that
 * can't be simulated from a pasted payload is the JWT itself: signature,
 * expiry and issuer are the real Auth Validator's job, so "auth" here
 * only checks that the payload is a JSON object, which is what a token
 * whose payload isn't would fail with (401).
 *
 * Every policy and filter is reported, not just the deciding one, so the
 * UI can show "matched but no grant here" / "disabled, skipped" per row.
 */

export type SimulationInput = {
  /** The decoded JWT payload as pasted -- raw text, parsed here. */
  payload: string;
  mcp: string;
  tool: string;
  endpoints: MCPRegistration[];
  accessPolicies: AccessPolicy[];
  filterPolicies: FilterPolicy[];
};

export type PolicyEvaluation = {
  name: string;
  enabled: boolean;
  /** null when the match was never evaluated: the policy is disabled
   * (Engine.Authorize skips it first), or auth already failed. */
  match: MatcherEvaluation | null;
  /** Whether any of the policy's grants covers (mcp, tool) -- reported
   * even for a non-matching or disabled policy, so the UI can explain
   * *which* half is missing. */
  covers: boolean;
  /** enabled && matched && covers: this policy authorizes the call. */
  grants: boolean;
};

export type FilterEvaluation = {
  name: string;
  enabled: boolean;
  /** null when never evaluated: disabled, or the request ended earlier. */
  match: MatcherEvaluation | null;
  applied: boolean;
  drop_fields: string[];
};

export type Outcome =
  | { kind: "invalid_payload"; status: 401; reason: string }
  | { kind: "policy_error"; status: 500; reason: string }
  | { kind: "forbidden"; status: 403; code: "forbidden" }
  | { kind: "mcp_disabled"; status: 403; code: "mcp_disabled" }
  | { kind: "mcp_not_found"; status: 404; code: "mcp_not_found" }
  | { kind: "tool_not_found"; status: 404; code: "tool_not_found" }
  | { kind: "ok"; status: 200 };

export type Simulation = {
  /** The parsed payload, or null when it isn't a JSON object. */
  claims: Record<string, unknown> | null;
  outcome: Outcome;
  /** Every access policy, in engine order. */
  policies: PolicyEvaluation[];
  authorized: boolean;
  /** Only the filters targeting exactly (mcp, tool) -- the rest never
   * enter FieldsToRemove's consideration at all. */
  filters: FilterEvaluation[];
  /** Union of the applied filters' drop_fields, deduplicated in order of
   * first appearance (Engine.FieldsToRemove). Empty unless the request
   * reached the response filter. */
  droppedFields: string[];
};

function grantCovers(grant: Grant, mcp: string, tool: string): boolean {
  if (grant.mcp !== "*" && grant.mcp !== mcp) return false;
  return grant.tools.some((t) => t === "*" || t === tool);
}

function parseClaims(payload: string): Record<string, unknown> | string {
  let parsed: unknown;
  try {
    parsed = JSON.parse(payload);
  } catch (err) {
    return `not valid JSON: ${err instanceof Error ? err.message : String(err)}`;
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    return "a JWT payload must be a JSON object";
  }
  return parsed as Record<string, unknown>;
}

export function simulate(input: SimulationInput): Simulation {
  const { mcp, tool } = input;
  const targetedFilters = input.filterPolicies.filter((f) => f.mcp === mcp && f.tool === tool);
  const unevaluatedFilters = (): FilterEvaluation[] =>
    targetedFilters.map((f) => ({ name: f.name, enabled: f.enabled, match: null, applied: false, drop_fields: f.drop_fields }));
  const unevaluatedPolicies = (): PolicyEvaluation[] =>
    input.accessPolicies.map((p) => ({
      name: p.name,
      enabled: p.enabled,
      match: null,
      covers: p.grants.some((g) => grantCovers(g, mcp, tool)),
      grants: false,
    }));

  // Stage 1: auth.
  const parsed = parseClaims(input.payload);
  if (typeof parsed === "string") {
    return {
      claims: null,
      outcome: { kind: "invalid_payload", status: 401, reason: parsed },
      policies: unevaluatedPolicies(),
      authorized: false,
      filters: unevaluatedFilters(),
      droppedFields: [],
    };
  }
  const claims = parsed;

  // Stage 2: access (Engine.Authorize).
  const policies: PolicyEvaluation[] = input.accessPolicies.map((p) => {
    const covers = p.grants.some((g) => grantCovers(g, mcp, tool));
    if (!p.enabled) return { name: p.name, enabled: false, match: null, covers, grants: false };
    const match = evaluateMatcher(claims, p.match);
    return { name: p.name, enabled: true, match, covers, grants: match.matched && covers };
  });
  const policyError = policies.find((p) => p.match?.error !== undefined);
  if (policyError) {
    return {
      claims,
      outcome: { kind: "policy_error", status: 500, reason: `access policy ${policyError.name}: ${policyError.match?.error}` },
      policies,
      authorized: false,
      filters: unevaluatedFilters(),
      droppedFields: [],
    };
  }
  const authorized = policies.some((p) => p.grants);
  const rejected = (outcome: Outcome): Simulation => ({
    claims,
    outcome,
    policies,
    authorized,
    filters: unevaluatedFilters(),
    droppedFields: [],
  });
  if (!authorized) return rejected({ kind: "forbidden", status: 403, code: "forbidden" });

  // Stage 3: endpoint + tool (resolver). Disabled is checked first, after
  // authorization, so its state is never disclosed to an unauthorized
  // caller (toolcall.go).
  const endpoint = input.endpoints.find((e) => e.name === mcp);
  if (endpoint && !endpoint.enabled) return rejected({ kind: "mcp_disabled", status: 403, code: "mcp_disabled" });
  if (!endpoint || endpoint.status !== "active") {
    return rejected({ kind: "mcp_not_found", status: 404, code: "mcp_not_found" });
  }
  if (!Object.hasOwn(endpoint.tools ?? {}, tool)) {
    return rejected({ kind: "tool_not_found", status: 404, code: "tool_not_found" });
  }

  // Stage 4: response filter (Engine.FieldsToRemove).
  const filters: FilterEvaluation[] = targetedFilters.map((f) => {
    if (!f.enabled) return { name: f.name, enabled: false, match: null, applied: false, drop_fields: f.drop_fields };
    const match = evaluateMatcher(claims, f.match);
    return { name: f.name, enabled: true, match, applied: match.matched, drop_fields: f.drop_fields };
  });
  const filterError = filters.find((f) => f.match?.error !== undefined);
  if (filterError) {
    return {
      claims,
      outcome: { kind: "policy_error", status: 500, reason: `filter policy ${filterError.name}: ${filterError.match?.error}` },
      policies,
      authorized,
      filters,
      droppedFields: [],
    };
  }
  const droppedFields = [...new Set(filters.filter((f) => f.applied).flatMap((f) => f.drop_fields))];

  return { claims, outcome: { kind: "ok", status: 200 }, policies, authorized, filters, droppedFields };
}

/**
 * A starting payload for the Request path tab's editor: a generic claim
 * set plus, for every rule simple enough to invert (a `$.name` or
 * `$["name"]` path, optionally `[*]`, and a pattern that is a literal
 * with or without `^…$` anchors), a claim that satisfies it -- so a
 * typical user's tab opens on the granted path instead of a 403 the
 * operator then has to edit their way out of. Rules that are real
 * regexps or deeper paths are skipped, not guessed at.
 */
export function suggestPayload(rules: ClaimRule[]): string {
  const claims: Record<string, unknown> = {
    sub: "svc-analytics",
    iss: "https://idp.internal",
    email: "analytics@acme.io",
    exp: 1788392000,
  };
  for (const rule of rules) {
    const key = simpleKey(rule.path);
    const literal = literalOf(rule.pattern);
    if (key === null || literal === null) continue;
    claims[key.name] = key.list ? [literal] : literal;
  }
  return JSON.stringify(claims, null, 2);
}

function simpleKey(path: string): { name: string; list: boolean } | null {
  const dotted = /^\$\.([A-Za-z0-9_\-\u0080-\uFFFF]+)(\[\*\])?$/.exec(path);
  if (dotted) return { name: dotted[1], list: dotted[2] !== undefined };
  const bracketed = /^\$\[(?:"([^"\\]+)"|'([^'\\]+)')\](\[\*\])?$/.exec(path);
  if (bracketed) return { name: bracketed[1] ?? bracketed[2], list: bracketed[3] !== undefined };
  return null;
}

function literalOf(pattern: string): string | null {
  const inner = pattern.replace(/^\^/, "").replace(/\$$/, "");
  if (inner === "" || /[.*+?()[\]{}|\\^$]/.test(inner)) return null;
  return inner;
}
