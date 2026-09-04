# ADR-0002: JSONPath + Regexp Claim Rule Engine

- Status: Accepted
- Date: 2026-09-04

## Context

The existing scaffold's routing and filtering rules (`router.RoutingRule`,
`filter.FieldFilterRule`, and `config.RoutingRule`/`FilteringRule`) match claims by
flat, exact string equality: `ClaimKey`/`ClaimValue`. This only works for
single-level, scalar claims (`role == "db-writer"`).

Real OIDC claim sets are not always flat scalars: group membership is commonly a
list (`groups: ["oncall-db", "team-data-eng"]`), and organizations often nest claims
under a namespace (`https://example.com/org.tier`). The design brief requires:

- Extracting a claim value by path, not just by top-level key.
- Matching that value against a pattern, not just exact equality (e.g. `^db-.*$`).
- Handling the case where the extracted value is a list: match if **any** element in
  the list satisfies the pattern (e.g. "is the caller in a group matching
  `oncall-.*`").

This is a foundational decision because both access policies and filter policies
(ADR-0004) are built directly on whatever rule shape is chosen here — it needs to be
right before either policy type is implemented.

## Decision

Represent every claim condition as a `ClaimRule{ Path string; Pattern string }`,
where `Path` is a JSONPath expression evaluated against the decoded JWT claim set (as
a generic JSON document, not a `map[string]string`), and `Pattern` is a Go `regexp`
evaluated against the string form of whatever `Path` extracts:

- If `Path` resolves to a single scalar, apply `regexp.MatchString(Pattern, value)`
  directly.
- If `Path` resolves to a list (e.g. via a wildcard segment like `$.groups[*]`),
  apply the regexp to each element and return true if **any** element matches. There
  is one evaluation function for both cases — the list case is not a separate rule
  type, only a different JSONPath result shape.
- If `Path` resolves to nothing (claim absent), the rule does not match; this is not
  an error.

A `ClaimMatcher` is a list of `ClaimRule`s combined with AND — every rule must match
for the matcher to match. OR is achieved by defining multiple policies (see
[ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md)), each with its own
matcher, rather than by adding OR to the rule language itself.

## Alternatives Considered

### Alternative A: Flat key/value equality (status quo)

Advantages:
- Already scaffolded; trivial to implement and reason about.

Disadvantages:
- Cannot express "any element of a list matches a pattern," which the design brief
  requires explicitly.
- Cannot reach nested or namespaced claims.
- Every new matching need (prefix match, list membership) would require a new ad hoc
  field on the rule struct, growing the config schema indefinitely.

### Alternative B: JSONPath + regexp (chosen)

Advantages:
- One rule shape covers scalar equality (`pattern: "^db-writer$"`), prefix/suffix
  matching, and list-membership-by-pattern, with no special-casing in config.
- JSONPath reaches nested/namespaced claims without inventing a separate
  path-flattening convention.
- Both requirements from the design brief (path extraction, list "any match") are
  satisfied by a single evaluation function.

Disadvantages:
- Requires a JSONPath library dependency (e.g. `github.com/PaesslerAG/jsonpath` or
  similar) — one more dependency than the status quo.
- Slightly more expensive per-request than a map lookup (JSONPath parse/evaluate +
  regexp compile). Mitigated by pre-compiling both the JSONPath expression and the
  regexp once at config-load time, not per request.
- Operators writing policy config need to know basic JSONPath syntax, a small
  increase in configuration complexity versus flat key/value pairs.

### Alternative C: A full policy DSL / expression language (e.g. CEL, OPA/Rego)

Advantages:
- Much more expressive — arbitrary boolean logic, comparisons, functions, in a single
  rule.

Disadvantages:
- Materially larger dependency and a new language for operators to learn, for a
  requirement (path extraction + pattern match, with list "any" semantics) that
  JSONPath + regexp already covers completely.
- Harder to reason about and audit for an air-gapped security-sensitive deployment
  than two well-understood primitives (JSONPath, regexp).

## Decision Criteria

- Functional fit against the two explicit requirements (path extraction; list "any
  element matches" semantics).
- Operational simplicity for operators writing policy YAML.
- Dependency footprint and auditability (security-sensitive, air-gapped deployments).
- Performance (evaluated per request, potentially against multiple policies).

## Rationale

JSONPath + regexp is the smallest rule language that satisfies both stated
requirements exactly, without introducing a general-purpose expression evaluator that
the project does not otherwise need. It keeps two well-known, independently
auditable primitives instead of one opaque DSL, which matters for a security-relevant
component. The single dependency it adds (a JSONPath evaluator) is a reasonable and
reversible cost.

## Consequences

### Positive

- One rule shape (`ClaimRule`) is reused unchanged for both access policies and
  filter policies (ADR-0004), keeping the policy engine small.
- Operators can express prefix, suffix, exact, and list-membership matching with the
  same two fields (`path`, `pattern`).

### Negative

- Adds a JSONPath library dependency to the `router`/policy-engine code.
- Misconfigured JSONPath expressions or regexps fail at rule-evaluation time unless
  caught at config load; mitigated by validating (parsing) every `Path` and
  compiling every `Pattern` during `config.Validate()`, not at request time.

### Risks

- A pathological regexp (e.g. catastrophic backtracking) evaluated per request could
  become a latency/DoS concern. Mitigated by using Go's `regexp` package, which is
  RE2-based and does not backtrack, so this class of risk does not apply.

### Follow-up

- Pick and vet a specific JSONPath library during implementation (must support
  wildcard segments like `$.groups[*]`); record the concrete choice in
  `config`/`router` code comments if it has notable behavioral quirks, rather than a
  separate ADR unless it turns out to be a meaningfully hard trade-off.

## Validation

Unit tests in `router` covering: scalar match, scalar non-match, list-any-match,
list-no-match, missing claim, nested/namespaced path — before this is relied on by
ADR-0004's policy engine.

## References

- [data.md](../data.md#claim-rule)
- [ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md)
