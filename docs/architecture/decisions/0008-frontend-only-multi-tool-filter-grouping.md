# ADR-0008: Frontend-Only Multi-Tool Filter Grouping via a `<name>::<tool>` Naming Convention

- Status: Accepted
- Date: 2026-09-05

## Context

`FilterPolicy` (see [data.md](../data.md)) targets exactly one `(mcp, tool)` pair —
`filter.Strip()` matches and applies `DropFields` against a single tool's response
shape, and there is no `tools []string` or wildcard form. This is a deliberate,
narrow data model (ADR-0004 keeps access and filtering as two auditable lists of
independently-scoped rules).

The admin webui's response-filter picker (Epic #76, Task #79), however, merges every
tool an MCP endpoint exposes into one searchable field list — an operator picking
fields to hide naturally thinks in terms of "hide `$.content` on this endpoint," not
"hide `$.content` on `create_directory`, then again on `directory_tree`, then again on
`edit_file`." This is especially visible on MCPs where several tools expose an
identically-named field (e.g. `@modelcontextprotocol/server-filesystem`'s 14 tools
each returning a bare `$.content`): building N separate single-tool filters by hand
for what is conceptually one policy is repetitive and easy to get inconsistent.

We need a way for one filter, as the operator experiences it, to span several tools —
without changing `filter.Strip()`'s matching logic, which is out of scope for a
webui-only task and would require its own ADR and backend migration.

## Decision

A "filter across N tools" is a frontend-only concept. Creating or editing one
produces N real, independent `FilterPolicy` records — one per tool with at least one
dropped field — whose `name` shares a `<group>::<tool>` prefix (`lib/filterGroups.ts`,
`GROUP_SEP = "::"`). The backend never sees or needs to know about the convention;
each record is a completely ordinary single-tool `FilterPolicy`.

The webui's "Response filters" list groups persisted records by the part of their
`name` before the first `::` (`groupIdOf`) and renders one card per group. Saving a
group diffs the desired per-tool field selection against the group's currently
persisted members (`planGroupSave`) and issues only the API calls a change actually
needs: an existing member keeps its exact `name` (`PUT`, never renamed), a newly
added tool mints a `<group>::<tool>` name (`POST`), and a tool removed from the
selection entirely is deleted (`DELETE`). A legacy or hand-crafted record with no
`::` in its name is treated as its own single-member group (`id` = the whole name) —
no migration needed, and this is also what any filter created directly against the
API (bypassing the webui) looks like.

The free-text filter-name input rejects a name containing `::` outright (inline
validation error, submit disabled) rather than silently stripping or escaping it —
see Consequences/Risks below.

## Alternatives Considered

### Alternative A: Extend `FilterPolicy` to accept multiple tools (`tool` → `tools []string`)

Advantages:
- One real backend record per logical filter — no frontend-only naming convention to
  maintain or explain.
- `GET /admin/filter-policies` would directly reflect what the operator created,
  with no client-side reconstruction step.

Disadvantages:
- Requires changing `filter.Strip()`'s matching signature and every caller, a
  persistence migration, and re-validating the "no wildcards, always exact" property
  ADR-0004 deliberately chose for filter policies — real backend work with its own
  design questions (e.g., do different tools' `drop_fields` share one list, or does
  the field list vary per tool within the same record?), out of scope for a webui
  task and not requested by the epic.
- A multi-tool `FilterPolicy` would need its own JSONPath semantics per tool (two
  tools rarely share a response shape), likely ending up as a map keyed by tool
  anyway — structurally not far from N single-tool records, just moved server-side.

### Alternative B: Frontend-only grouping via a name-prefix convention (chosen)

Advantages:
- Zero backend changes; ships entirely within the webui task.
- Every record remains an ordinary, independently valid `FilterPolicy` — `curl`ing
  the admin API directly still works exactly as documented, with no awareness of
  the grouping convention required.
- Reversible: deleting the convention later (should the backend ever grow real
  multi-tool support) means simply not minting `::`-suffixed names going forward;
  existing suffixed records still work as N independent policies either way.

Disadvantages:
- The grouping is purely a display/editing convenience — `GET /admin/filter-policies`
  shows N records for what the operator experiences as one filter, which could
  confuse someone auditing policies directly against the API without knowing this
  convention exists (mitigated by this ADR and the inline code comments in
  `lib/filterGroups.ts`).
- A `name` string doing double duty as both an identifier and a grouping key is a
  narrow but real footgun: an operator-chosen name containing `::` collides with an
  existing group sharing that prefix (see Risks).

### Alternative C: Client-side-only storage of the grouping (e.g. a browser-local mapping of group → member names)

Advantages:
- Avoids overloading `name` entirely; the real record names stay opaque/arbitrary.

Disadvantages:
- This is an admin tool used by multiple operators from different browsers — a
  purely client-local mapping would only be visible to whoever created it, on the
  machine/browser they created it from, and would be lost on a cache clear. Rejected
  outright during the same design discussion that produced this ADR, for the same
  reason `docs/CONFIG.md` requires the admin API itself to be the source of truth.

## Decision Criteria

- No backend changes required (task scope: webui only).
- Every persisted record stays independently valid and inspectable via the
  documented admin API, with no hidden client-only state.
- Reversibility if the backend later grows real multi-tool support.

## Rationale

Alternative B is the only option that ships within the webui task's actual scope,
keeps the backend's already-deliberate single-tool `FilterPolicy` model untouched,
and keeps the admin API itself (not browser storage) as the single source of truth —
consistent with `docs/CONFIG.md`'s framing of the admin API as the trusted surface.
Alternative A is very plausibly the *right* long-term design if multi-tool filters
turn out to be a common real need, but it's a backend/data-model decision that
deserves its own ADR and isn't required to unblock the current task.

## Consequences

### Positive

- Operators build and edit filters the way they naturally think about them (one
  filter, several tools) without any backend change.
- `planGroupSave`'s create/update/delete diffing means editing a group only ever
  touches the records that actually changed — no blind delete-and-recreate of the
  whole group on every save.

### Negative

- Two different mental models coexist: the admin API's "N independent records" and
  the webui's "one filter." Anyone extending the webui's filter UI needs to read
  this ADR (or `lib/filterGroups.ts`'s doc comments) to understand why a `name`
  sometimes contains `::`.

### Risks

- **Name-collision / cross-group contamination.** Found via security audit
  (2026-09-05, same day this convention shipped) and fixed the same day: nothing
  stopped an operator from typing a name that itself contains `::` (e.g.
  `billing::extra`). Since `groupIdOf` groups by the text before the *first* `::`,
  the resulting record (`billing::extra::get_user`) would silently group under an
  unrelated pre-existing `billing` group's card — and editing that merged card could
  issue `PUT`/`DELETE` against the other filter's real records via `planGroupSave`,
  since it diffs against the full (accidentally merged) member list. Fixed by
  rejecting `::` in the name input at creation time (`ResponseFilterGroup.tsx`) —
  see `lib/filterGroups.test.ts`'s documentation test for `groupIdOf`'s intentional
  first-occurrence behavior, which is what makes the input-time rejection necessary
  rather than optional.
- **Tool names are not validated anywhere on the backend** (`list_tools()`'s output
  is trusted as-is — see [ADR-0003](0003-dynamic-mcp-registration-and-schema-discovery.md)).
  A related bug (same audit, same day) was the webui's internal per-tool field key
  using a plain space as a delimiter, which a tool name itself containing a space
  would break; fixed by JSON-encoding the `(tool, path)` pair instead of
  delimiter-joining it. Not a grouping-convention risk specifically, but the same
  audit pass and the same root cause (a human-controllable string used as part of a
  delimiter-joined key) — noted here since it directly informed the `::`-rejection
  decision above rather than a "pick a safer delimiter" one.

### Follow-up

- If multi-tool filters become common enough that N-records-per-filter becomes an
  operational nuisance (e.g. hard to bulk-audit, or the record count becomes large),
  revisit Alternative A as a real backend change — this ADR's convention should be
  explicitly superseded, not left running alongside a new backend feature doing the
  same thing two ways.

## Validation

`lib/filterGroups.test.ts` (pure logic: grouping, diffing/reconciliation, the
first-`::`-occurrence documentation test) and `ResponseFilterGroup.test.tsx`
(component-level: multi-tool creation issuing N real `onCreate` calls sharing a
prefix, editing a group reconciling create/update/delete against its real members,
the `::`-in-name input rejection). Also verified end-to-end against a real running
gateway + real MCP (`@modelcontextprotocol/server-filesystem`, 14 tools sharing a
`$.content` field) via headless Chromium: a two-tool filter created through the UI
produces exactly two real records sharing a name prefix, shown as one card, and
editing it to swap one tool for another issues exactly one `PUT` (unchanged tool,
same name), one `POST` (newly added tool), one `DELETE` (removed tool).

## References

- [data.md](../data.md) — `FilterPolicy` shape.
- [ADR-0004](0004-unified-policy-engine-for-access-and-filtering.md) — why
  `FilterPolicy` is single-tool, exact-match, in the first place.
- [api/admin.md](../../api/admin.md#filter-policies) — the real API every generated
  record is an ordinary instance of.
- `gateway/internal/controlplane/webui/src/lib/filterGroups.ts` — implementation.
