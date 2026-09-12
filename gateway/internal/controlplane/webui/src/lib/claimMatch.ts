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
 * - JSON numbers outside float64's exact range print differently.
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
};

export type MatcherEvaluation = {
  matched: boolean;
  rules: RuleEvaluation[];
  /** The first rule error, if any -- the matcher as a whole can't be
   * trusted when one of its rules is unusable. */
  error?: string;
};

/** Go's `stringify`: a string as-is, null as "", everything else via
 * fmt.Sprint -- which for JSON scalars is what String() gives too. */
export function stringifyClaim(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
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
    if (err instanceof JSONPathError) return { matched: false, found: [], error: `invalid path: ${err.message}` };
    throw err;
  }
  return { matched: found.some((node) => matchNode(node, pattern)), found };
}

/** Go's ClaimMatcher.Matches: every rule must match; no rules matches
 * everything. Unlike Go (which stops at the first failure), every rule is
 * evaluated so the UI can show each one's own pass/fail. */
export function evaluateMatcher(claims: unknown, rules: ClaimRule[]): MatcherEvaluation {
  const evaluated = rules.map((rule) => evaluateRule(claims, rule));
  const error = evaluated.find((r) => r.error !== undefined)?.error;
  return { matched: error === undefined && evaluated.every((r) => r.matched), rules: evaluated, error };
}
