import { useEffect, useMemo, useRef, useState } from "react";
import { statusFromString } from "../primitives/StatusDot";
import { simulate, suggestPayload, type Simulation } from "../../lib/requestPath";
import type { AccessPolicy, FilterPolicy, MCPRegistration } from "../../api/types";
import type { User } from "../../api/users";
import { groupIdOf, stripGroupPrefix } from "../../lib/filterGroups";

export type RequestPathTabProps = {
  /** The user as saved -- the simulation runs against persisted policy,
   * never the panel's draft (see `draftDirty`). */
  user: User;
  endpoints: MCPRegistration[];
  /** Every access policy, not just this user's: Engine.Authorize is a
   * union over all of them, so a claim set can be granted by another
   * policy even when this user's own doesn't match. */
  allAccessPolicies: AccessPolicy[];
  allFilters: FilterPolicy[];
  /** True while Token match / Access hold unsaved edits, which this
   * simulation deliberately doesn't see. */
  draftDirty: boolean;
  onGoAccess: () => void;
  onGoInstances: () => void;
};

type Sel = "request" | "client" | "auth" | "access" | "cache" | "filter" | `ep:${string}`;

type Tone = "ink" | "ok" | "danger" | "warn" | "accent" | "muted";
const toneText: Record<Tone, string> = {
  ink: "text-ink",
  ok: "text-success",
  danger: "text-danger",
  warn: "text-warn",
  accent: "text-accent",
  muted: "text-muted",
};
const toneSoft: Record<Tone, string> = {
  ink: "bg-surface",
  ok: "bg-success-soft",
  danger: "bg-danger-bg",
  warn: "bg-warn-soft",
  accent: "bg-select-soft",
  muted: "bg-bg",
};

type Row = { k: string; v: string; tone?: Tone; soft?: boolean };
type Section = { label: string; rows: Row[] };
type Detail = {
  kind: string;
  title: string;
  badge: string;
  badgeTone: Tone;
  note: string;
  sections: Section[];
  cta?: { label: string; onClick: () => void };
};

// The mockup's canvas: a fixed 1160x620 drawing inside 24px padding,
// scaled down as one unit to fit the panel's width (see the host's
// ResizeObserver below) rather than reflowed.
const CANVAS_W = 1208;
const CANVAS_H = 668;
const ROW_H = 58;
const MAX_ROWS = 7;
const MAX_GRANTED_ROWS = 4;

type Edge = { d: string; stroke: string; w: number; dash: string; anim: string };

// Raw hex here on purpose: these are SVG stroke values, which Tailwind's
// utility classes don't reach. They're the mockup's `C` constants
// (theme.css's --color-* tokens carry the same values for the rest).
const STROKE = { line: "#cfd6e0", base: "#e0e4ea", dim: "#e8ebf0", static: "#dfe3ea", accent: "#2f4bd8", danger: "#b3261e" };

function userGrantCovers(user: User, endpointName: string): boolean {
  return user.grants.some((g) => g.mcp === "*" || g.mcp === endpointName);
}

function filterLabel(name: string): string {
  // `<user>::<label>::<tool>` -> `<label>` (see AccessTab / ADR-0008).
  const parts = stripGroupPrefix(name, groupIdOf(name)).split("::");
  return parts.length > 1 ? parts.slice(0, -1).join("::") : parts[0];
}

function plural(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`;
}

/**
 * The third Users & access tab (#81): a client-side simulation of the
 * auth → access → endpoint → response-filter pipeline for one request,
 * computed by lib/requestPath.ts against the policies as saved and the
 * claim payload / target call the operator picks here. Nothing is sent
 * to the gateway -- there is no preview endpoint by design (see the
 * frontend design doc's Non-goals), and this is not an audit log.
 *
 * Layout follows the real mockup source (fetched via DesignSync): the
 * scenario pill bar it uses to fake four states is replaced by the two
 * inputs that actually determine the state here (payload + target
 * call); the graph (Client → Auth Validator → Access Check → endpoint
 * column → Response Filter, Schema Cache above) keeps the mockup's exact
 * coordinates, edges and animations; the detail panel below keeps its
 * kind/title/badge/note/sections/CTA shape with real decisions in it.
 *
 * Reject treatment per the design brief: a 401 lights Auth Validator, a
 * 403/404/500 lights Access Check (or the endpoint row, for endpoint-
 * stage rejections) in the danger palette with the alert pulse, and the
 * request badge goes red -- meant to be unmissable, not decorative.
 */
export function RequestPathTab({
  user,
  endpoints,
  allAccessPolicies,
  allFilters,
  draftDirty,
  onGoAccess,
  onGoInstances,
}: RequestPathTabProps) {
  const granted = endpoints.filter((e) => userGrantCovers(user, e.name));
  const others = endpoints.filter((e) => !userGrantCovers(user, e.name));

  const defaultTarget = useMemo(() => {
    const ep = granted[0] ?? endpoints[0];
    if (!ep) return null;
    const grant = user.grants.find((g) => g.mcp === "*" || g.mcp === ep.name);
    const tools = Object.keys(ep.tools ?? {});
    const tool = (grant && !grant.tools.includes("*") && grant.tools.find((t) => tools.includes(t))) || tools[0] || "";
    return { mcp: ep.name, tool };
    // Only for the initial pick; the operator's own choice wins after.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const [payload, setPayload] = useState(() => suggestPayload(user.match));
  const [target, setTarget] = useState(defaultTarget);
  const [sel, setSel] = useState<Sel>("request");

  const hostRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const ro = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width ?? CANVAS_W;
      setScale(Math.min(1, width / CANVAS_W));
    });
    ro.observe(host);
    return () => ro.disconnect();
  }, []);

  const targetEndpoint = target ? endpoints.find((e) => e.name === target.mcp) : undefined;
  const targetTools = Object.keys(targetEndpoint?.tools ?? {});

  const sim: Simulation | null = useMemo(
    () =>
      target
        ? simulate({
            payload,
            mcp: target.mcp,
            tool: target.tool,
            endpoints,
            accessPolicies: allAccessPolicies,
            filterPolicies: allFilters,
          })
        : null,
    [payload, target, endpoints, allAccessPolicies, allFilters],
  );

  if (endpoints.length === 0 || !target || !sim) {
    return (
      <div className="p-4">
        <div className="flex items-center justify-between gap-3 mb-3 text-[11.5px] text-muted">
          <span>Simulation against the policies as currently saved · not a live request trace or an audit log</span>
        </div>
        <div className="max-w-[248px] p-[18px_16px] border border-dashed border-line rounded-xl bg-surface flex flex-col gap-2.5 items-start">
          <div className="text-[12.5px] font-semibold">No MCP endpoints</div>
          <div className="text-[11.5px] text-subtle leading-[1.45]">
            Nothing to grant and nothing to call. Add an endpoint to start serving traffic.
          </div>
          <button
            type="button"
            onClick={onGoInstances}
            className="flex items-center gap-2 px-2.5 py-2 border border-dashed border-line rounded-[9px] bg-surface cursor-pointer text-left w-full hover:border-accent hover:bg-form-soft"
          >
            <span className="w-4 h-4 rounded-[5px] border border-dashed border-line grid place-items-center text-[11px] text-muted">
              +
            </span>
            <span className="text-xs font-semibold text-ink">Add MCP endpoint</span>
          </button>
        </div>
      </div>
    );
  }

  // ---- scenario, derived from the real outcome (the mockup fakes these with pills) ----
  const outcome = sim.outcome;
  const reject = outcome.kind === "invalid_payload";
  const live = outcome.kind === "ok";
  const accessDenied = !reject && !live && !sim.authorized;
  const endpointDenied = !reject && !live && sim.authorized && outcome.kind !== "policy_error";
  const filterError = !reject && !live && sim.authorized && outcome.kind === "policy_error";
  const denied = accessDenied || endpointDenied || filterError;

  // ---- endpoint column: granted first (max 4), then others, target always shown ----
  const shown = (() => {
    const list = [...granted.slice(0, MAX_GRANTED_ROWS), ...others.slice(0, Math.max(0, MAX_ROWS - Math.min(MAX_GRANTED_ROWS, granted.length)))];
    if (targetEndpoint && !list.includes(targetEndpoint)) list[list.length - 1] = targetEndpoint;
    return list;
  })();
  const targetIndex = targetEndpoint ? shown.indexOf(targetEndpoint) : -1;
  const rowY = (i: number) => 120 + ROW_H * i;

  const edges: Edge[] = (() => {
    const out: Edge[] = [
      { d: "M516,56 V268", stroke: STROKE.static, w: 1.4, dash: "4 5", anim: "none" },
      { d: "M602,32 H1006 V268", stroke: STROKE.static, w: 1.4, dash: "4 5", anim: "none" },
    ];
    const base = (d: string): Edge => ({ d, stroke: STROKE.line, w: 1.5, dash: "none", anim: "none" });
    const dim = (d: string): Edge => ({ d, stroke: STROKE.dim, w: 1.5, dash: "none", anim: "none" });
    const flow = (d: string, color: string): Edge => ({ d, stroke: color, w: 2, dash: "7 9", anim: "dashflow .9s linear infinite" });
    const toRow = (i: number) => `M602,300 H626 V${rowY(i)} H652`;
    const fromRow = (i: number) => `M868,${rowY(i)} H892 V300 H920`;

    if (reject) {
      out.push(flow("M154,300 H206", STROKE.danger), dim("M378,300 H430"));
      shown.forEach((_, i) => out.push(dim(toRow(i)), dim(fromRow(i))));
      out.push(flow("M206,332 V500 H80 V332", STROKE.danger), dim("M1006,332 V590 H80 V332"));
      return out;
    }
    if (denied) {
      out.push(flow("M154,300 H206", STROKE.accent), flow("M378,300 H430", STROKE.accent));
      shown.forEach((_, i) => {
        out.push(i === targetIndex ? flow(toRow(i), STROKE.danger) : dim(toRow(i)));
        out.push(filterError && i === targetIndex ? flow(fromRow(i), STROKE.danger) : dim(fromRow(i)));
      });
      out.push(flow("M430,332 V500 H80 V332", STROKE.danger), dim("M1006,332 V590 H80 V332"));
      return out;
    }
    out.push(flow("M154,300 H206", STROKE.accent), flow("M378,300 H430", STROKE.accent));
    shown.forEach((_, i) => {
      out.push(i === targetIndex ? flow(toRow(i), STROKE.accent) : base(toRow(i)));
      out.push(i === targetIndex ? flow(fromRow(i), STROKE.accent) : base(fromRow(i)));
    });
    out.push(flow("M1006,332 V590 H80 V332", STROKE.accent));
    return out;
  })();

  // ---- node chrome ----
  const ringOf = (on: boolean, danger = false) =>
    on ? `0 0 0 3px ${danger ? "#fdecea" : "#eef1fe"}` : "0 1px 2px rgba(16,24,40,.05)";
  const nodeClass = (on: boolean, danger: boolean, dashed = false) =>
    `absolute p-[11px_13px] bg-surface rounded-[11px] cursor-pointer text-left border ${dashed ? "border-dashed" : ""} ${
      on ? "border-accent" : danger ? "border-danger" : dashed ? "border-line" : "border-border"
    }`;

  const grantingPolicies = sim.policies.filter((p) => p.grants).map((p) => p.name);
  const appliedFilters = sim.filters.filter((f) => f.applied);
  const statusOf = (e: MCPRegistration) => (e.enabled ? statusFromString(e.status) : "disabled");
  const dotClass = (e: MCPRegistration) =>
    !e.enabled ? "bg-track-off" : statusFromString(e.status) === "active" ? "bg-success" : "bg-warn";

  const authMeta = reject ? `401 · ${outcome.kind === "invalid_payload" ? "payload rejected" : ""}` : `verified · ${plural(sim.policies.filter((p) => p.match?.matched).length, "policy", "policies")} matched`;
  const routerMeta = reject
    ? "not reached"
    : accessDenied
      ? outcome.kind === "policy_error"
        ? "500 · policy error"
        : "403 · no grant"
      : `granted · ${target.mcp}`;
  const endpointMeta = (e: MCPRegistration) => {
    const isGranted = userGrantCovers(user, e.name);
    if (!isGranted) return "not granted";
    return `${plural(user.filters.filter((f) => f.mcp === e.name).length, "filter")} for ${user.name}`;
  };
  const filterMeta = live
    ? `${plural(appliedFilters.length, "filter")} ran`
    : filterError
      ? "500 · policy error"
      : "not reached";
  const cachedCount = endpoints.filter((e) => Object.keys(e.tools ?? {}).length > 0).length;

  // ---- detail panel ----
  const R = (k: string, v: string, tone?: Tone, soft = false): Row => ({ k, v, tone, soft });
  const outcomeBadge = (): [string, Tone] => {
    switch (outcome.kind) {
      case "ok":
        return ["200 · granted", "ok"];
      case "invalid_payload":
        return ["401 · rejected", "danger"];
      case "forbidden":
        return ["403 · denied", "danger"];
      case "mcp_disabled":
        return ["403 · mcp_disabled", "danger"];
      case "mcp_not_found":
        return ["404 · mcp_not_found", "danger"];
      case "tool_not_found":
        return ["404 · tool_not_found", "danger"];
      case "policy_error":
        return ["500 · policy error", "danger"];
    }
  };
  const traceRows = (): Row[] => {
    const rows: Row[] = [];
    rows.push(reject ? R("Auth Validator", `reject · 401 · ${outcome.kind === "invalid_payload" ? outcome.reason : ""}`, "danger", true) : R("Auth Validator", "verified · payload is a JSON object", "ok"));
    if (reject) rows.push(R("Access Check", "skipped", "muted"));
    else if (accessDenied) rows.push(R("Access Check", outcome.kind === "policy_error" ? `error · 500 · ${outcome.reason}` : "deny · 403 · no grant", "danger", true));
    else rows.push(R("Access Check", `granted · ${grantingPolicies.join(", ")}`, "ok"));
    const epLabel = target.mcp;
    if (reject || accessDenied) rows.push(R(epLabel, "skipped", "muted"));
    else if (outcome.kind === "mcp_disabled") rows.push(R(epLabel, "disabled · 403", "danger", true));
    else if (outcome.kind === "mcp_not_found") rows.push(R(epLabel, "not active · 404", "danger", true));
    else if (outcome.kind === "tool_not_found") rows.push(R(epLabel, `${target.tool} · 404 tool not found`, "danger", true));
    else rows.push(R(epLabel, `${target.tool} called`));
    if (live) rows.push(R("Response Filter", sim.droppedFields.length ? `${plural(sim.droppedFields.length, "field")} dropped` : "full response", sim.droppedFields.length ? "warn" : "muted"));
    else if (filterError) rows.push(R("Response Filter", `error · 500 · ${outcome.kind === "policy_error" ? outcome.reason : ""}`, "danger", true));
    else rows.push(R("Response Filter", "skipped", "muted"));
    return rows;
  };

  const detail = (): Detail => {
    const [badge, badgeTone] = outcomeBadge();
    if (sel.startsWith("ep:")) {
      const e = endpoints.find((x) => x.name === sel.slice(3));
      if (e) {
        const isGranted = userGrantCovers(user, e.name);
        const grant = user.grants.find((g) => g.mcp === "*" || g.mcp === e.name);
        const filters = user.filters.filter((f) => f.mcp === e.name);
        return {
          kind: "MCP endpoint",
          title: e.name,
          badge: isGranted ? "granted" : "not granted",
          badgeTone: isGranted ? "accent" : "muted",
          note: isGranted
            ? "This user has an explicit grant on this endpoint. Only the filters attached to that grant run on its response."
            : `${user.name} has no grant here — a request naming this endpoint is rejected with 403 after auth.`,
          sections: [
            {
              label: "Endpoint",
              rows: [
                R("transport", e.transport),
                R("connection", statusOf(e), !e.enabled ? "danger" : statusFromString(e.status) === "active" ? "ok" : "warn"),
                R("tools", String(Object.keys(e.tools ?? {}).length)),
              ],
            },
            {
              label: isGranted ? `Filters for ${user.name}` : "Access",
              rows: isGranted
                ? [
                    R("tools granted", grant?.tools.includes("*") ? "all" : (grant?.tools.join(", ") ?? "—"), "accent"),
                    ...(filters.length ? filters.map((f) => R(filterLabel(f.name), `${f.tool} · ${plural(f.drop_fields.length, "field")}`, "warn", true)) : [R("none", "full response", "warn", true)]),
                  ]
                : [R("grant", "missing", "danger", true), R("result", "403", "danger", true)],
            },
          ],
          cta: { label: isGranted ? "Edit this grant →" : "Grant access →", onClick: onGoAccess },
        };
      }
    }
    if (sel === "auth") {
      const own = sim.policies.find((p) => p.name === user.name);
      const ownRules = own?.match
        ? own.match.rules.map((r, i) =>
            R(`${user.match[i].path} ~ ${user.match[i].pattern}`, r.error ? `error · ${r.error}` : r.matched ? `✓ found: ${r.found.map((v) => JSON.stringify(v)).join(", ")}` : r.found.length ? `✕ found: ${r.found.map((v) => JSON.stringify(v)).join(", ")}` : "✕ not present", r.error ? "danger" : r.matched ? "ok" : "danger", !r.matched),
          )
        : [R("conditions", reject ? "not evaluated" : "skipped", "muted")];
      if (reject) {
        return {
          kind: "Pipeline node",
          title: "Auth Validator",
          badge: "Reject (401)",
          badgeTone: "danger",
          note: "The payload can't be read as a claim set, so nothing downstream ran — the gateway never looked at grants.",
          sections: [
            { label: "Failure", rows: [R("reason", outcome.kind === "invalid_payload" ? outcome.reason : "", "danger", true), R("status", "401", "danger", true)] },
            { label: "Not simulated", rows: [R("signature", "checked by the real validator", "muted"), R("exp / iss", "checked by the real validator", "muted")] },
          ],
        };
      }
      return {
        kind: "Pipeline node",
        title: "Auth Validator",
        badge: "Token verified",
        badgeTone: "ok",
        note: "Signature, expiry and issuer are the real validator's job and aren't simulated here. The payload is then matched against every policy's JSONPath/regex conditions.",
        sections: [
          { label: "Checks", rows: [R("payload", "valid JSON object", "ok"), R("signature · exp · iss", "not simulated", "muted")] },
          { label: `${user.name}'s conditions`, rows: ownRules.length ? ownRules : [R("none", "matches every token", "warn", true)] },
          {
            label: "Other policies",
            rows: sim.policies.filter((p) => p.name !== user.name).length
              ? sim.policies.filter((p) => p.name !== user.name).map((p) => R(p.name, !p.enabled ? "disabled · skipped" : p.match?.matched ? "matches" : "no match", !p.enabled ? "muted" : p.match?.matched ? "ok" : "muted"))
              : [R("none", "this is the only policy", "muted")],
          },
        ],
      };
    }
    if (sel === "access") {
      const policyRows = sim.policies.map((p) =>
        R(
          p.name,
          !p.enabled ? "disabled · skipped" : p.match === null ? "not evaluated" : p.match.error ? `error · ${p.match.error}` : p.grants ? "matches · grants this call" : p.match.matched ? "matches · no grant on this call" : "no match",
          !p.enabled ? "muted" : p.match?.error ? "danger" : p.grants ? "ok" : p.match?.matched ? "warn" : "muted",
          !!p.match?.error || (p.match?.matched && !p.grants),
        ),
      );
      if (reject) {
        return { kind: "Pipeline node", title: "Access Check", badge: "Not reached", badgeTone: "muted", note: "Grants are only checked after the token is verified. This request never got here.", sections: [] };
      }
      if (accessDenied) {
        return {
          kind: "Pipeline node",
          title: "Access Check",
          badge: outcome.kind === "policy_error" ? "Error (500)" : "Denied (403)",
          badgeTone: "danger",
          note:
            outcome.kind === "policy_error"
              ? `A saved rule can't be evaluated: ${outcome.reason}. The gateway fails the request rather than guess.`
              : "The token is valid, but no enabled policy both matches these claims and grants this endpoint + tool. There is no fallback rule — access is explicit.",
          sections: [
            { label: "Decision", rows: [R("endpoint asked", target.mcp, "danger", true), R("tool", target.tool, "danger", true), R("grant", "none", "danger", true), R("status", outcome.kind === "policy_error" ? "500" : "403", "danger", true)] },
            { label: "Policies", rows: policyRows },
          ],
          cta: { label: `Grant access to ${user.name} →`, onClick: onGoAccess },
        };
      }
      return {
        kind: "Pipeline node",
        title: "Access Check",
        badge: "Granted",
        badgeTone: "accent",
        note: "Access is an explicit grant for these claims on this endpoint — no claim routing, no priority order, no fallback.",
        sections: [
          { label: "Decision", rows: [R("endpoint", target.mcp, "accent"), R("tool", target.tool, "accent"), R("granted by", grantingPolicies.join(", "), "ok")] },
          { label: "Policies", rows: policyRows },
        ],
        cta: { label: `Edit access for ${user.name} →`, onClick: onGoAccess },
      };
    }
    if (sel === "filter") {
      if (!live && !filterError) {
        return { kind: "Pipeline node", title: "Response Filter", badge: "Not reached", badgeTone: "muted", note: "The request was rejected before any MCP was called, so there is no response to filter.", sections: [] };
      }
      const filterRows = sim.filters.length
        ? sim.filters.map((f) =>
            R(
              filterLabel(f.name),
              !f.enabled ? "disabled · skipped" : f.match?.error ? `error · ${f.match.error}` : f.applied ? `applied · ${plural(f.drop_fields.length, "field")}` : "no match · skipped",
              !f.enabled ? "muted" : f.match?.error ? "danger" : f.applied ? "warn" : "muted",
              f.applied || !!f.match?.error,
            ),
          )
        : [R("none", "no filter targets this endpoint + tool", "muted")];
      return {
        kind: "Pipeline node",
        title: "Response Filter",
        badge: filterError ? "Error (500)" : appliedFilters.length ? `${plural(appliedFilters.length, "filter")} ran` : "Full response",
        badgeTone: filterError ? "danger" : appliedFilters.length ? "warn" : "muted",
        note: filterError
          ? `A saved filter rule can't be evaluated: ${outcome.kind === "policy_error" ? outcome.reason : ""}.`
          : `Every enabled filter targeting ${target.mcp} / ${target.tool} whose conditions match these claims runs; their dropped fields are unioned.`,
        sections: [
          { label: "Filters on this call", rows: filterRows },
          { label: "Fields dropped", rows: sim.droppedFields.length ? sim.droppedFields.map((p) => R(p, "dropped", "warn", true)) : [R("none", "full response", "muted")] },
        ],
        cta: { label: "Edit filters on this grant →", onClick: onGoAccess },
      };
    }
    if (sel === "cache") {
      return {
        kind: "Pipeline node",
        title: "Schema Cache",
        badge: `${plural(cachedCount, "schema")}`,
        badgeTone: "muted",
        note: "One discovered tool set per endpoint, refreshed by the gateway. A filter can only strip fields the cache knows about.",
        sections: [{ label: "Coverage", rows: [R("endpoints with tools", `${cachedCount} of ${endpoints.length}`, cachedCount === endpoints.length ? "ok" : "warn"), R(`tools on ${target.mcp}`, String(targetTools.length))] }],
      };
    }
    if (sel === "client") {
      return {
        kind: "Pipeline node",
        title: "Client / Agent",
        badge: `${outcome.status} returned`,
        badgeTone: live ? "muted" : "danger",
        note: reject
          ? "The agent received a 401 with no MCP payload."
          : denied
            ? `The agent received a ${outcome.status} — authenticated, but the call didn't get through.`
            : "The agent received the endpoint's response, minus whatever the matching filters dropped.",
        sections: [{ label: "Session", rows: [R("user", user.name), R("granted endpoints", String(granted.length), granted.length ? "ink" : "danger")] }],
      };
    }
    return {
      kind: "Request",
      title: reject ? "Rejected request" : denied ? "Denied request" : "Simulated request",
      badge,
      badgeTone,
      note: "Select a node to see the decision it made for this request.",
      sections: [{ label: "Trace", rows: traceRows() }],
    };
  };
  const d = detail();

  const nodeButton = (id: Sel, label: string, style: React.CSSProperties, danger: boolean, children: React.ReactNode, extra: React.CSSProperties = {}, dashed = false) => {
    const on = sel === id;
    return (
      <button
        type="button"
        aria-label={label}
        aria-pressed={on}
        onClick={() => setSel(id)}
        className={nodeClass(on, danger, dashed)}
        style={{ ...style, boxShadow: dashed && !on ? "none" : ringOf(on, danger), ...extra }}
      >
        {children}
      </button>
    );
  };

  return (
    <div className="flex flex-col">
      <div className="grid grid-cols-[repeat(auto-fit,minmax(300px,1fr))] gap-px bg-border-soft border-b border-border-soft">
        <div className="bg-surface min-w-0 p-3.5 flex flex-col gap-2">
          <div className="flex items-baseline gap-2">
            <span className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">Decoded JWT payload</span>
            <span className="flex-1" />
            <span className="text-[10.5px] text-muted">any OIDC provider</span>
          </div>
          <textarea
            aria-label="Decoded JWT payload"
            value={payload}
            onChange={(e) => setPayload(e.target.value)}
            spellCheck={false}
            className="h-[170px] p-[11px] border border-border rounded-[9px] bg-well font-mono text-[11.5px] leading-[1.6] text-ink resize-none outline-none focus:border-accent"
          />
          {reject && (
            <div className="px-[11px] py-[9px] rounded-lg bg-warn-soft text-[11px] text-warn">
              Not valid JSON — fix the payload to evaluate conditions.
            </div>
          )}
        </div>
        <div className="bg-surface min-w-0 p-3.5 flex flex-col gap-2">
          <div className="flex items-baseline gap-2">
            <span className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">Simulated call</span>
            <span className="flex-1" />
            <span className="text-[10.5px] text-muted">
              MCP endpoints · {granted.length} granted to {user.name}
            </span>
          </div>
          <label className="flex flex-col gap-1 text-[10.5px] text-subtle">
            Endpoint
            <select
              aria-label="Endpoint"
              value={target.mcp}
              onChange={(e) => {
                const ep = endpoints.find((x) => x.name === e.target.value);
                const tools = Object.keys(ep?.tools ?? {});
                setTarget({ mcp: e.target.value, tool: tools[0] ?? "" });
              }}
              className="px-2.5 py-2 border border-border rounded-lg bg-surface text-[12.5px] text-ink font-mono outline-none focus:border-accent"
            >
              {endpoints.map((e) => (
                <option key={e.name} value={e.name}>
                  {e.name}
                  {userGrantCovers(user, e.name) ? "" : " (not granted)"}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-[10.5px] text-subtle">
            Tool
            <select
              aria-label="Tool"
              value={target.tool}
              onChange={(e) => setTarget({ mcp: target.mcp, tool: e.target.value })}
              className="px-2.5 py-2 border border-border rounded-lg bg-surface text-[12.5px] text-ink font-mono outline-none focus:border-accent"
            >
              {targetTools.length === 0 && <option value="">(no tools discovered)</option>}
              {targetTools.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>
          <p className="m-0 text-[11px] text-muted leading-[1.5]">
            Simulation against the policies as currently saved — not a live request trace or an audit log. Signature,
            expiry and issuer are not checked here.
          </p>
          {draftDirty && (
            <p className="m-0 px-[11px] py-[9px] rounded-lg bg-warn-soft text-[11px] text-warn leading-[1.5]">
              Unsaved changes on Token match / Access aren't reflected until you save.
            </p>
          )}
        </div>
      </div>

      <div
        ref={hostRef}
        className="overflow-hidden border-b border-border-soft bg-bg"
        style={{ backgroundImage: "radial-gradient(#dfe3ea 1px, transparent 1px)", backgroundSize: "22px 22px", height: CANVAS_H * scale }}
      >
        <div style={{ width: CANVAS_W, height: CANVAS_H, padding: 24, transformOrigin: "top left", transform: `scale(${scale})` }}>
          <div className="relative" style={{ width: 1160, height: 620 }}>
            <svg viewBox="0 0 1160 620" width="1160" height="620" className="absolute inset-0 overflow-visible" aria-hidden="true">
              {edges.map((e, i) => (
                <path key={i} d={e.d} fill="none" stroke={e.stroke} strokeWidth={e.w} strokeDasharray={e.dash} strokeLinecap="round" style={{ animation: e.anim }} />
              ))}
            </svg>

            {nodeButton(
              "client",
              "Client / Agent",
              { left: 4, top: 268, width: 150 },
              false,
              <>
                <div className="text-[10px] font-semibold tracking-[.07em] uppercase text-muted">Client / Agent</div>
                <div className="mt-[5px] text-[13px] font-semibold">{user.name}</div>
                <div className="mt-[3px] text-[11px] text-subtle font-mono">{live ? "response streamed" : `${outcome.status} returned`}</div>
              </>,
            )}

            {nodeButton(
              "auth",
              "Auth Validator",
              { left: 206, top: 268, width: 172 },
              reject,
              <>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-[13px] font-semibold">Auth Validator</div>
                  <span className={`w-[7px] h-[7px] rounded-full ${reject ? "bg-danger" : "bg-success"}`} />
                </div>
                <div className={`mt-1 text-[11px] font-mono ${reject ? "text-danger" : "text-muted"}`}>{authMeta}</div>
                <div className="mt-[7px] text-[11px] text-subtle">JWT signature · exp · issuer</div>
              </>,
              { animation: reject ? "rejectpulse 1.6s ease-in-out infinite" : "none" },
            )}

            {nodeButton(
              "access",
              "Access Check",
              { left: 430, top: 268, width: 172 },
              accessDenied,
              <>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-[13px] font-semibold">Access Check</div>
                  <span className={`w-[7px] h-[7px] rounded-full ${reject ? "bg-track-off" : accessDenied ? "bg-danger" : "bg-success"}`} />
                </div>
                <div className={`mt-1 text-[11px] font-mono ${reject ? "text-muted" : accessDenied ? "text-danger" : "text-accent"}`}>{routerMeta}</div>
                <div className="mt-[7px] text-[11px] text-subtle">
                  {plural(user.grants.length, "grant")} for {user.name}
                </div>
              </>,
              { animation: accessDenied ? "rejectpulse 1.6s ease-in-out infinite" : live ? "softpulse 1.6s ease-in-out infinite" : "none" },
            )}

            {nodeButton(
              "cache",
              "Schema Cache",
              { left: 430, top: 8, width: 172, padding: "10px 13px" },
              false,
              <>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-[12.5px] font-semibold">Schema Cache</div>
                  <span className="text-[10px] text-muted font-mono">read</span>
                </div>
                <div className="mt-[3px] text-[11px] text-subtle font-mono">{plural(cachedCount, "schema")}</div>
              </>,
              {},
              true,
            )}

            <div className="absolute text-[10px] font-semibold tracking-[.07em] uppercase text-muted" style={{ left: 652, top: 62 }}>
              MCP endpoints · {granted.length} granted to {user.name}
            </div>
            {shown.map((e, i) => {
              const isGranted = userGrantCovers(user, e.name);
              const hit = i === targetIndex;
              const hot = hit && !reject;
              const rowDanger = hit && (endpointDenied || accessDenied);
              const on = sel === `ep:${e.name}`;
              return (
                <button
                  key={e.name}
                  type="button"
                  aria-label={`MCP endpoint ${e.name}`}
                  aria-pressed={on}
                  onClick={() => setSel(`ep:${e.name}`)}
                  className={`absolute h-12 px-3 bg-surface rounded-[10px] flex items-center justify-between gap-2 cursor-pointer text-left border ${
                    on ? "border-accent" : rowDanger ? "border-danger" : hot || isGranted ? "border-accent" : "border-border"
                  }`}
                  style={{
                    left: 652,
                    width: 216,
                    top: 96 + ROW_H * i,
                    boxShadow: on ? "0 0 0 3px #eef1fe" : hot ? `0 0 0 3px ${rowDanger ? "#fdecea" : "#eef1fe"}` : "0 1px 2px rgba(16,24,40,.05)",
                    opacity: reject ? 0.5 : isGranted || hit ? 1 : 0.55,
                    animation: endpointDenied && hit ? "rejectpulse 1.6s ease-in-out infinite" : "none",
                  }}
                >
                  <div className="flex flex-col gap-0.5 min-w-0">
                    <div className="text-xs font-semibold truncate">{e.name}</div>
                    <div className="text-[10px] text-subtle font-mono truncate">{endpointMeta(e)}</div>
                  </div>
                  <div className="flex items-center gap-[7px] shrink-0">
                    <span className={`px-[7px] py-0.5 rounded-[20px] text-[10px] font-semibold ${isGranted ? "bg-select-soft text-accent" : "bg-bg text-muted"}`}>
                      {isGranted ? "granted" : "—"}
                    </span>
                    <span className={`w-[7px] h-[7px] rounded-full ${dotClass(e)}`} />
                  </div>
                </button>
              );
            })}
            <div className="absolute text-[10.5px] text-muted" style={{ left: 652, top: 560, width: 216 }}>
              {endpoints.length} endpoints total · {endpoints.length - shown.length} not shown
            </div>

            {nodeButton(
              "filter",
              "Response Filter",
              { left: 920, top: 268, width: 172 },
              filterError,
              <>
                <div className="flex items-center justify-between gap-2">
                  <div className="text-[13px] font-semibold">Response Filter</div>
                  <span className={`w-[7px] h-[7px] rounded-full ${!live ? (filterError ? "bg-danger" : "bg-track-off") : appliedFilters.length ? "bg-warn" : "bg-success"}`} />
                </div>
                <div className={`mt-1 text-[11px] font-mono ${!live ? (filterError ? "text-danger" : "text-muted") : appliedFilters.length ? "text-warn" : "text-muted"}`}>{filterMeta}</div>
                <div className="mt-[7px] text-[11px] text-subtle">filters on this call only</div>
              </>,
              { animation: filterError ? "rejectpulse 1.6s ease-in-out infinite" : "none" },
            )}

            <div className="absolute text-[11px] text-muted" style={{ left: 430, top: 552 }}>
              filtered response → Client / Agent
            </div>
          </div>
        </div>
      </div>

      <div data-testid="path-detail" className="bg-well p-4 grid grid-cols-[repeat(auto-fit,minmax(250px,1fr))] gap-3.5 items-start">
        <div className="flex items-start justify-between gap-2.5">
          <div className="flex flex-col gap-[3px]">
            <div className="text-[10px] font-semibold tracking-[.07em] uppercase text-muted">{d.kind}</div>
            <div className="text-[15.5px] font-semibold tracking-tight">{d.title}</div>
          </div>
          <span data-testid="path-badge" className={`px-[9px] py-[3px] rounded-[20px] text-[11px] font-semibold whitespace-nowrap ${toneText[d.badgeTone]} ${toneSoft[d.badgeTone]}`}>
            {d.badge}
          </span>
        </div>
        <div className="text-[12.5px] text-subtle leading-[1.5] -mt-1.5">{d.note}</div>

        {d.sections.map((s) => (
          <div key={s.label} className="flex flex-col gap-2">
            <div className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">{s.label}</div>
            <div className="border border-border rounded-[10px] overflow-hidden">
              {s.rows.map((r, i) => (
                <div key={`${r.k}-${i}`} className={`flex items-baseline justify-between gap-3 px-3 py-[9px] border-b border-border-soft last:border-b-0 ${r.soft && r.tone ? toneSoft[r.tone] : "bg-surface"}`}>
                  <div className="text-xs text-subtle whitespace-nowrap">{r.k}</div>
                  <div className={`text-xs font-medium text-right font-mono ${toneText[r.tone ?? "ink"]}`}>{r.v}</div>
                </div>
              ))}
            </div>
          </div>
        ))}

        {d.cta && (
          <button
            type="button"
            onClick={d.cta.onClick}
            className="mt-0.5 px-3 py-[9px] border border-border rounded-[9px] bg-surface text-[12.5px] font-medium cursor-pointer text-left hover:bg-bg"
          >
            {d.cta.label}
          </button>
        )}
      </div>
    </div>
  );
}
