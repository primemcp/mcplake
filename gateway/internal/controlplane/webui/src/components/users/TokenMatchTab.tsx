import { Input } from "../primitives/Input";
import type { ClaimRule } from "../../api/types";

export type TokenMatchTabProps = {
  match: ClaimRule[];
  onChange: (match: ClaimRule[]) => void;
};

/**
 * Edits the `ClaimRule[]` shared by a user's `AccessPolicy` and every one
 * of their `FilterPolicy` records (kept in sync on save — see
 * UsersScreen). The mockup's own Token match tab also has a live
 * pass/fail preview against a decoded JWT payload, but that needs the
 * same claim-matching reimplementation #81 (Request path tab) is scoped
 * to build — this is the editor only for now; the preview lands there.
 *
 * `router.ClaimMatcher.Matches` with zero rules matches *everything* (see
 * router/claimrule.go's own doc comment) -- so a user with no conditions
 * isn't unreachable, it's the opposite: it resolves for every token,
 * granting whatever it grants to anyone. The empty-state copy below (and
 * UserDetail's Save gate requiring at least one condition) reflects that
 * real semantics, not the inverted "can never be resolved" framing an
 * earlier pass of this file had.
 */
export function TokenMatchTab({ match, onChange }: TokenMatchTabProps) {
  const setRule = (index: number, patch: Partial<ClaimRule>) => {
    onChange(match.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  };
  const removeRule = (index: number) => {
    onChange(match.filter((_, i) => i !== index));
  };
  const addRule = () => {
    onChange([...match, { path: "$.", pattern: "" }]);
  };

  return (
    <div className="flex flex-col gap-2.5">
      <div className="flex items-baseline gap-2">
        <span className="text-[10.5px] font-semibold tracking-wide uppercase text-muted">Conditions</span>
        <span className="text-[10.5px] text-muted">
          {match.length} · AND
        </span>
      </div>
      {match.length === 0 ? (
        <p className="text-[11.5px] text-warn border border-dashed border-line rounded-[10px] p-3">
          No condition yet — this resolves for every token, granting whatever it grants to anyone. Add at
          least one JSONPath / regex pair to scope it.
        </p>
      ) : (
        <div className="flex flex-col gap-1.5">
          {match.map((rule, i) => (
            <div key={i} className="flex items-center gap-1.5">
              <Input
                mono
                value={rule.path}
                onChange={(e) => setRule(i, { path: e.target.value })}
                placeholder="$.claim.path"
                aria-label={`Condition ${i + 1} path`}
                className="flex-1"
              />
              <span className="text-[11px] text-muted shrink-0">matches</span>
              <Input
                mono
                value={rule.pattern}
                onChange={(e) => setRule(i, { pattern: e.target.value })}
                placeholder="^value$"
                aria-label={`Condition ${i + 1} regex`}
                className="flex-1"
              />
              <button
                type="button"
                aria-label={`Remove condition ${i + 1}`}
                onClick={() => removeRule(i)}
                className="shrink-0 border-0 bg-transparent cursor-pointer text-danger text-sm p-1 leading-none"
              >
                ✕
              </button>
            </div>
          ))}
        </div>
      )}
      <button
        type="button"
        onClick={addRule}
        className="self-start flex items-center gap-1.5 px-2.5 py-1.5 border border-dashed border-line rounded-lg bg-surface cursor-pointer text-[11.5px] text-body"
      >
        <span className="w-3.5 h-3.5 rounded border border-dashed border-muted grid place-items-center text-[10px] text-muted">
          +
        </span>
        Add condition
      </button>
    </div>
  );
}
