import { describe, expect, it } from "vitest";
import fixture from "../../../../../../testdata/claim_stringify_cases.json";
import { evaluateMatcher, evaluateRule, stringifyClaim } from "./claimMatch";

// Verbatim from router/claimrule_test.go's sampleClaims, so every case
// below has a Go twin with the same expected answer.
const sampleClaims = {
  role: "db-writer",
  sub: "user-123",
  groups: ["oncall-db", "team-data-eng"],
  empty_groups: [],
  "https://example.com/org": { tier: "enterprise" },
  id: 42,
};

describe("evaluateRule", () => {
  // Mirrors TestClaimRule_Matches case for case.
  it.each<[string, string, string, boolean]>([
    ["scalar match", "$.role", "^db-writer$", true],
    ["scalar non-match", "$.role", "^db-reader$", false],
    ["scalar prefix match", "$.role", "^db-.*$", true],
    ["list-any-match via wildcard", "$.groups[*]", "^oncall-.*$", true],
    ["list-any-match via whole array", "$.groups", "^oncall-.*$", true],
    ["list-no-match", "$.groups[*]", "^admin-.*$", false],
    ["empty list never matches", "$.empty_groups[*]", ".*", false],
    ["missing claim path is not a match", "$.nonexistent", ".*", false],
    ["nested namespaced path", '$["https://example.com/org"].tier', "^enterprise$", true],
    ["nested namespaced path non-match", '$["https://example.com/org"].tier', "^free$", false],
    ["numeric claim stringified", "$.id", "^42$", true],
  ])("%s", (_name, path, pattern, want) => {
    const got = evaluateRule(sampleClaims, { path, pattern });

    expect(got.error).toBeUndefined();
    expect(got.matched).toBe(want);
  });

  it("reports the values the path selected, for the UI to show as 'found: …'", () => {
    expect(evaluateRule(sampleClaims, { path: "$.groups[*]", pattern: "x" }).found).toEqual([
      "oncall-db",
      "team-data-eng",
    ]);
    expect(evaluateRule(sampleClaims, { path: "$.nonexistent", pattern: "x" }).found).toEqual([]);
  });

  it("a whole-array selection is matched element by element, one level deep -- a nested array is stringified, not flattened", () => {
    const claims = { nested: [["a", "b"], "c"] };

    expect(evaluateRule(claims, { path: "$.nested", pattern: "^c$" }).matched).toBe(true);
    expect(evaluateRule(claims, { path: "$.nested", pattern: "^a$" }).matched).toBe(false);
  });

  it("a null claim value matches as the empty string, like Go's stringify", () => {
    expect(evaluateRule({ role: null }, { path: "$.role", pattern: "^$" }).matched).toBe(true);
  });

  it("the pattern is an unanchored search, like regexp.MatchString", () => {
    expect(evaluateRule(sampleClaims, { path: "$.role", pattern: "writer" }).matched).toBe(true);
  });

  // Go rejects these at NewClaimRule time, before any request is
  // evaluated; here they surface as a per-rule error the UI can show
  // instead of a misleading pass/fail.
  it("reports a malformed path as an error, not a non-match", () => {
    const got = evaluateRule(sampleClaims, { path: "not a valid jsonpath $$", pattern: "^ok$" });

    expect(got.matched).toBe(false);
    expect(got.error).toMatch(/path/i);
  });

  it("reports a malformed pattern as an error, not a non-match", () => {
    const got = evaluateRule(sampleClaims, { path: "$.role", pattern: "(unclosed" });

    expect(got.matched).toBe(false);
    expect(got.error).toMatch(/pattern/i);
  });
});

describe("evaluateMatcher", () => {
  // Mirrors TestClaimMatcher_Matches.
  it("an empty matcher matches everything", () => {
    expect(evaluateMatcher(sampleClaims, []).matched).toBe(true);
  });

  it("a single matching rule matches", () => {
    expect(evaluateMatcher(sampleClaims, [{ path: "$.role", pattern: "^db-writer$" }]).matched).toBe(true);
  });

  it("a single non-matching rule fails", () => {
    expect(evaluateMatcher(sampleClaims, [{ path: "$.role", pattern: "^db-reader$" }]).matched).toBe(false);
  });

  it("all rules must match (AND)", () => {
    const rules = [
      { path: "$.role", pattern: "^db-writer$" },
      { path: "$.groups[*]", pattern: "^oncall-.*$" },
    ];
    expect(evaluateMatcher(sampleClaims, rules).matched).toBe(true);
  });

  it("one failing rule fails the matcher (AND)", () => {
    const rules = [
      { path: "$.role", pattern: "^db-writer$" },
      { path: "$.groups[*]", pattern: "^admin-.*$" },
    ];
    const got = evaluateMatcher(sampleClaims, rules);

    expect(got.matched).toBe(false);
    expect(got.rules.map((r) => r.matched)).toEqual([true, false]);
  });

  it("a rule error fails the matcher and is surfaced on the matcher", () => {
    const got = evaluateMatcher(sampleClaims, [
      { path: "$.role", pattern: "^db-writer$" },
      { path: "$.role", pattern: "(unclosed" },
    ]);

    expect(got.matched).toBe(false);
    expect(got.error).toMatch(/pattern/i);
  });
});

// The shared fixture router/stringify_parity_test.go also reads. One table
// for two implementations, so they cannot drift apart again: before #162
// this side rendered 1000042 as "1000042" while the gateway rendered
// "1.000042e+06", and a filter policy anchored on the former reported a
// redaction the gateway never performed.
type StringifyCase = { name: string; value: unknown; want: string };
// The fixture lives at the repository root so both language toolchains can
// reach it; router/stringify_parity_test.go reads the same file.
const stringifyCases: StringifyCase[] = fixture.cases;

describe("stringifyClaim", () => {
  it("has the shared fixture loaded", () => {
    expect(stringifyCases.length).toBeGreaterThan(10);
  });

  it.each(stringifyCases.map((c) => [c.name, c.value, c.want] as const))(
    "%s: renders %j as %j, byte-identically to Go's stringify",
    (_name, value, want) => {
      expect(stringifyClaim(value)).toBe(want);
    },
  );

  // A rule anchored on the gateway's rendering must match here too --
  // stringifyClaim is only interesting insofar as evaluateRule agrees.
  it.each(stringifyCases.map((c) => [c.name, c.value, c.want] as const))(
    "%s: a rule anchored on the gateway's rendering matches",
    (_name, value, want) => {
      const pattern = `^${want.replace(/[.+*?()[\]{}^$|\\]/g, "\\$&")}$`;

      expect(evaluateRule({ claim: value }, { path: "$.claim", pattern }).matched).toBe(true);
    },
  );

  it("renders a JSON object as JSON rather than as Go's map syntax (a documented, harmless difference)", () => {
    expect(stringifyClaim({ a: 1 })).toBe('{"a":1}');
  });
});
