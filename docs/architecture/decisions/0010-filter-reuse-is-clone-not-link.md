# ADR-0010: Reusing an existing response filter clones it, it doesn't link it

- Status: Accepted
- Date: 2026-09-09

## Context

The Users & access screen's Access tab lets an operator attach response
filters (dropped fields) to a user's grant on an MCP endpoint. The design
mockup shows this as picking from a list of the endpoint's *existing*
filters (checkbox-select), implying one shared filter object usable across
several users.

The real backend has no concept of a filter "belonging to" a user or being
shared across several. `router.FilterPolicy` (`router/filterpolicy.go`) is
evaluated purely on its own `Match` + `(MCP, Tool)`, independent of any
`AccessPolicy`:

```go
// FieldsToRemove returns the union ... of DropFields from every
// FilterPolicy whose Match matches claims and whose (MCP, Tool) equals
// the requested pair.
```

There is no `Owner`/`AccessPolicyName` field on `FilterPolicy`, and
`router.ClaimMatcher` only supports AND across its rules (ADR-0002) — there
is no way to express "matches user A OR user B" in a single `Match`. So one
physical `FilterPolicy` genuinely cannot correctly serve two users whose
match conditions differ: each user needs their own record, carrying their
own match, or the filter will silently fail to apply to (or incorrectly
apply to) the wrong caller.

The admin webui already gives each user's filters their own real
`FilterPolicy` records, named `<user>::<mcp>::<tool>` (ADR-0008's grouping
convention, generalized in #80) — this is bookkeeping for *display and
CRUD reconciliation* in the UI only, not a backend relationship.

## Decision

"Attach an existing filter" in the Access tab's response-filter picker
means: clone that filter's `drop_fields` as the *starting values* for a
new (or edited) `FilterPolicy` record that belongs to the current user
(carries the current user's own `match`). It is a one-time copy, not a
live link — editing the source filter afterward does not propagate to
anything cloned from it, and there is no persisted association between
the two records beyond that one copy operation.

## Alternatives Considered

### A: Linked/shared filter group

Add a `TemplateID` (or similar) to track that several `FilterPolicy`
records were derived from a common source, and offer a "these N records
are linked — propagate this edit?" flow when one is edited.

Advantages:
- Matches the mockup's implied "one shared filter, multiple owners" UX
  more literally.
- Editing a widely-used filter once, in one place, is less repetitive.

Disadvantages:
- Introduces a genuinely new correctness question the codebase doesn't
  have today: "is this copy still in sync with its template?" — every
  answer (silent drift, forced sync, block-until-resolved, warn-only)
  is a new state machine and a new class of bug (partial-failure during
  bulk propagate, concurrent-edit races, orphaned template).
- Doesn't even fully solve the original problem: it only covers filters
  created *through* this linking flow. A filter registered directly on
  the MCP connections screen (broad `match`, not tied to any user) is
  just as real and just as "shared" in effect, and needs no such link at
  all — so the linking machinery only ever covers a subset of cases.
- No backend representation for "linked" exists or is proposed; this
  would live entirely as extra frontend bookkeeping layered onto records
  the backend still treats as fully independent.

### B: Real backend ownership field

Give `FilterPolicy` an explicit link to the `AccessPolicy` it belongs to,
and have `Engine.FieldsToRemove` only apply a linked filter when the
request was *also* authorized under that specific `AccessPolicy`.

Advantages:
- A real, enforced relationship — no more relying on two independently
  evaluated `Match` values happening to agree.
- "Used by N users" becomes exact instead of a heuristic.

Disadvantages:
- Changes the filtering engine's semantics: two previously orthogonal
  concepts (ADR-0004's independent `Authorize`/`FieldsToRemove` passes)
  become coupled for linked filters, with new edge cases (the linked
  `AccessPolicy` gets deleted; the link points at a disabled policy).
- Real backend surface: new column, migration, engine logic, API
  surface, docs — for a project still in early phases per the repo's own
  `CLAUDE.md` phase framing.
- Still doesn't cover MCP-connections-created filters, same as
  Alternative A.
- A separate, bigger piece of work with its own trade-offs; worth its
  own ADR if pursued later, not bundled into this one.

## Decision Criteria

- **Correctness**: clone-not-link keeps every `FilterPolicy` record
  self-contained and truthful about what it actually does (its own
  `match` + `drop_fields`) — nothing to silently drift out of sync.
- **Simplicity**: no new data model, no new UI state beyond "prefill a
  form from an existing record's fields," which the field picker already
  supports via its `initial` prop.
- **Maintainability**: zero new edge cases to test and document forever,
  versus an open-ended set for either alternative.
- **Reliability**: no distributed-state consistency problem to get
  wrong under concurrent edits or partial failures.

## Rationale

Given the project's stated priority order (correctness, then simplicity,
then maintainability, ahead of literal mockup fidelity — see
`.claude/skills/architecture`), clone-not-link wins on every axis that
matters here, and the mockup's apparent "shared filter" UX doesn't
actually correspond to something the backend can enforce correctly today
regardless of which alternative is chosen. Revisiting Alternative B is
reasonable once there's real, repeated pain from filters drifting out of
sync in practice — not before.

## Consequences

### Positive

- The response-filter picker gets a genuinely useful "start from an
  existing filter" convenience without any new backend work or new
  correctness surface.
- Every `FilterPolicy` record stays independently correct and
  understandable in isolation — reading one record never requires
  knowing about any other.

### Negative

- Editing a filter's fields for one user never updates any other user's
  copy that started from the same template — an operator managing many
  structurally-similar users has to repeat the edit for each one.

### Risks

- If this repetition becomes a real operational pain point, revisit
  Alternative B (or a lighter version of A) rather than accumulating ad
  hoc bulk-edit tooling around the clone model.

### Follow-up

- None planned. Track real-world friction before reopening this.

## References

- ADR-0002 (JSONPath+regexp claim-rule engine — AND-only `ClaimMatcher`)
- ADR-0004 (unified policy engine — `Authorize`/`FieldsToRemove` as
  independent passes)
- ADR-0008 (frontend-only multi-tool filter grouping — the
  `<name>::...` naming convention this ADR builds on)
- `router/filterpolicy.go`, `router/claimrule.go`
