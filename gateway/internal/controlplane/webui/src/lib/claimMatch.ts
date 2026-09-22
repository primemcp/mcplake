import type { ClaimRule } from "../api/types";
import { JSONPathError, selectPath } from "./jsonpath";

/**
 * A TypeScript twin of `router.ClaimRule.Matches` / `router.ClaimMatcher.
 * Matches` (router/claimrule.go), so the Request path tab can simulate
 * what the gateway's policy engine decides for a pasted claim set
 * without a backend round trip (#81). Kept deliberately structure-for-
 * structure with the Go code: every branch here has a counterpart there,
 * and claimMatch.test.ts mirrors router/claimrule_test.go case for case.
 *
 * Known, accepted differences from Go (none reachable by a well-formed
 * rule against a real JWT payload):
 * - Regexps are JavaScript `RegExp`, not RE2. Anchors, classes, groups
 *   and alternation behave the same; RE2-only syntax like `(?P<name>…)`
 *   would error here and match there.
 * - A rule that selects an object or nested array stringifies it as JSON
 *   here and as Go's `fmt.Sprint` (`map[k:v]`) there. Both are "the rule
 *   targets a non-scalar", which no sane pattern matches either way.
 *
 * Numbers are *not* on that list: they are reproduced exactly, and
 * testdata/claim_stringify_cases.json is the shared fixture both this
 * implementation and router/claimrule_test.go's Go one are checked
 * against. They used to disagree for every value at or above 1e6, which
 * made the Request path tab report redactions the gateway never performed
 * (#162).
 */

export type RuleEvaluation = {
  matched: boolean;
  /** Every value the rule's path selected -- what the UI shows as
   * "found: …". Empty when the claim is absent. */
  found: unknown[];
  /** Set when the rule itself is unusable (malformed path or pattern);
   * `matched` is false in that case. Go rejects such rules at
   * construction (NewClaimRule) rather than at evaluation. */
  error?: string;
  /** True when `error` means "this simulator cannot evaluate the rule",
   * not "the rule is broken". The gateway evaluates such a rule normally,
   * so the caller must not report it as a gateway failure. */
  unsupported?: boolean;
};

export type MatcherEvaluation = {
  matched: boolean;
  rules: RuleEvaluation[];
  /** The first rule error, if any -- the matcher as a whole can't be
   * trusted when one of its rules is unusable. */
  error?: string;
  /** True when every error here is merely unsupported by this simulator.
   * The distinction decides whether the caller shows a gateway error or a
   * "could not be simulated" caveat. */
  unsupported?: boolean;
};

/**
 * Go's `stringify`: a string as-is, null as "", everything else through
 * `fmt.Sprint`.
 *
 * For numbers that is emphatically not `String(value)`. `fmt.Sprint` on a
 * float64 -- which is what `json.Unmarshal` into `any` always produces --
 * is `%v` → `%g` → `strconv.FormatFloat(x, 'g', -1, 64)`, so see
 * formatLikeGoFloat for the part that differs.
 */
export function stringifyClaim(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "";
  if (typeof value === "number") return formatLikeGoFloat(value);
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

/**
 * Renders a number the way Go renders a float64 with `%v`.
 *
 * Go uses the shortest representation that round-trips, in scientific
 * notation when the decimal exponent is `< -4` or `>= 6`
 * (`strconv/ftoa.go`: for shortest formatting the comparison precision is
 * fixed at 6). JavaScript's `String()` only switches at 1e21, so
 * everything from 1e6 up -- unix timestamps, tenant ids, account numbers
 * -- rendered differently on the two sides.
 *
 * The exponent is read back from `toExponential()` rather than computed
 * with `Math.log10`, which is off by one at exact powers of ten
 * (`Math.log10(1000)` is 2.9999999999999996). `toExponential()` with no
 * argument also yields the same shortest round-tripping digits Go picks,
 * so only the exponent's formatting has to be adjusted: Go pads it to at
 * least two digits and always writes the sign.
 */
function formatLikeGoFloat(value: number): string {
  // Go prints negative zero as "-0"; String() collapses it to "0".
  if (Object.is(value, -0)) return "-0";
  if (!Number.isFinite(value) || value === 0) return String(value);

  const [mantissa, exponent] = value.toExponential().split("e");
  const exp = Number(exponent);
  if (exp >= -4 && exp < 6) {
    return String(value);
  }
  const sign = exp < 0 ? "-" : "+";
  return `${mantissa}e${sign}${String(Math.abs(exp)).padStart(2, "0")}`;
}

function matchNode(node: unknown, pattern: RegExp): boolean {
  // Go's matchNode: a selected array is tested element by element (the
  // "$.groups" rather than "$.groups[*]" case), exactly one level deep.
  if (Array.isArray(node)) return node.some((elem) => pattern.test(stringifyClaim(elem)));
  return pattern.test(stringifyClaim(node));
}

export function evaluateRule(claims: unknown, rule: ClaimRule): RuleEvaluation {
  let pattern: RegExp;
  try {
    pattern = new RegExp(rule.pattern);
  } catch (err) {
    return { matched: false, found: [], error: `invalid pattern: ${err instanceof Error ? err.message : String(err)}` };
  }
  let found: unknown[];
  try {
    found = selectPath(claims, rule.path);
  } catch (err) {
    if (err instanceof JSONPathError) {
      const label = err.unsupported ? "cannot be simulated" : "invalid path";
      return { matched: false, found: [], error: `${label}: ${err.message}`, unsupported: err.unsupported };
    }
    throw err;
  }
  return { matched: found.some((node) => matchNode(node, pattern)), found };
}

/** Go's ClaimMatcher.Matches: every rule must match; no rules matches
 * everything. Unlike Go (which stops at the first failure), every rule is
 * evaluated so the UI can show each one's own pass/fail. */
export function evaluateMatcher(claims: unknown, rules: ClaimRule[]): MatcherEvaluation {
  const evaluated = rules.map((rule) => evaluateRule(claims, rule));
  const failed = evaluated.filter((r) => r.error !== undefined);
  const error = failed[0]?.error;
  return {
    matched: error === undefined && evaluated.every((r) => r.matched),
    rules: evaluated,
    error,
    unsupported: failed.length > 0 && failed.every((r) => r.unsupported === true),
  };
}
