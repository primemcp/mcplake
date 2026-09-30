ADR-0004: One Claim-Rule Engine for Both Access Control and Response Filtering
==============================================================================

:Status: Accepted
:Date: 2026-09-04

Context
-------

The design brief specifies a pipeline of the shape:

::

   JWT eval -> [ mcps ] -> [ tools ]
   tool -> resp_filter -> JWT eval

That is: JWT evaluation gates which ``(mcp, tool)`` pairs a caller may invoke, and the
*same kind* of JWT evaluation is run again after the tool call to decide which
response fields to strip. The current scaffold already has two separate rule types —
``router.RoutingRule`` and ``filter.FieldFilterRule`` — that both key off
``ClaimKey``/``ClaimValue``, but they are independent structs with independent matching
code, and the field-filter rule additionally overloads its meaning: today's
``FieldFilterRule`` both selects *when* a filter applies (via claim/mcp/tool) and *what*
it removes (``HideFields``), duplicating the same claim-check logic the router already
has.

We need to decide whether access control and response filtering share one matching
engine or remain two independent implementations, now that both are being rebuilt on
top of the JSONPath+regexp rule model (ADR-0002).

Decision
--------

Both access policies and filter policies are matched by the exact same
``ClaimMatcher``/``ClaimRule`` evaluator from ADR-0002. The only difference between the
two policy kinds is their payload on a match:

- ``AccessPolicy.Grants`` — which ``(mcp, tool)`` pairs become callable.
- ``FilterPolicy.DropFields`` — which response field paths get removed, scoped to one
  ``(mcp, tool)``.

Concretely, the Policy Engine (living in ``router``, per
:ref:`architecture-components-policy-engine-router-new-claim-rule-matching`)
exposes two entry points built on one internal ``matches(claims, ClaimMatcher) bool``:

.. code-block:: go

   func (e *Engine) Authorize(claims Claims, mcp, tool string) (bool, error)
   func (e *Engine) FieldsToRemove(claims Claims, mcp, tool string) ([]string, error)

``Authorize`` is true if any ``AccessPolicy`` matches and grants ``(mcp, tool)``.
``FieldsToRemove`` is the union of ``DropFields`` from every ``FilterPolicy`` that matches
and targets ``(mcp, tool)``. Both policy kinds are loaded from the same config file,
declared as separate lists (``access_policies``, ``filter_policies`` — see
:doc:`/architecture/data`) so that access and filtering remain independently auditable,
even though they share evaluation code.

Alternatives Considered
-----------------------

Alternative A: Keep access and filtering as separate matching implementations (status quo shape)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- No shared abstraction to design; each module owns its own simple matcher.

Disadvantages:

- Directly duplicates the claim-matching logic that ADR-0002 already had to solve
  once; a bug fix or extension (e.g. a new JSONPath edge case) would need to be
  applied twice.
- Does not reflect the design brief's stated pipeline, which explicitly reuses "JWT
  eval" for both stages.

Alternative B: One shared ``ClaimMatcher`` engine, two policy payloads (chosen)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Matches the design brief's pipeline directly: one evaluation mechanism, consulted
  twice.
- Single implementation to test for claim-matching correctness; ``AccessPolicy`` and
  ``FilterPolicy`` differ only in their trailing data, not in how they're matched.
- New policy kinds in the future (if ever needed) could reuse the same matcher again.

Disadvantages:

- Couples the two features to one shared abstraction — a bug in ``ClaimMatcher``
  affects both authorization and filtering simultaneously, rather than being
  isolated to one.

Alternative C: Merge access and filtering into a single policy object (one policy = grants + drop fields together)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Fewer config lists to author for the common case where the same audience gets both
  a grant and a filtering rule.

Disadvantages:

- Conflates two independent concerns (what can be called vs. what's visible in the
  result) into one object, which is harder to audit — a reviewer checking "who can
  see salary data" would have to read every access policy's filtering side-effects
  instead of one dedicated list. Field filtering also frequently applies across
  multiple access policies (e.g. every non-admin role hides the same PII fields),
  which this alternative would force to be duplicated per access policy instead of
  declared once.

Decision Criteria
-----------------

- Fidelity to the specified pipeline (JWT eval reused for both stages).
- Avoiding duplicated matching logic (maintainability).
- Auditability of "who can call what" separately from "what gets hidden."

Rationale
---------

Alternative B is the direct implementation of the pipeline as specified, avoids
duplicating ADR-0002's matching logic, and keeps access and filtering independently
readable in config while sharing one tested evaluation path. Alternative C's config
convenience is outweighed by the auditability cost of mixing two different security
concerns into one object.

Consequences
------------

Positive
~~~~~~~~

- A single, well-tested ``ClaimMatcher`` evaluator backs every authorization and
  filtering decision in the gateway.
- Access and filter policies can be reviewed as two independent lists, matching how
  security reviewers typically reason about these two questions separately.

Negative
~~~~~~~~

- Every tool call now runs claim-rule evaluation twice (once for ``Authorize``, once
  for ``FieldsToRemove``) instead of once; mitigated by the engine being pure/stateless
  and the underlying JSONPath/regexp being pre-compiled (ADR-0002), so the marginal
  cost of the second evaluation is small relative to the downstream MCP call.

Risks
~~~~~

- If ``FieldsToRemove`` silently returns nothing (e.g. due to a config typo in ``mcp``/
  ``tool`` matching), sensitive fields could leak with no error surfaced. Mitigated by
  ``config.Validate()`` checking that every ``FilterPolicy.MCP``/``Tool`` refers to a
  registered MCP/tool at load time (for statically configured MCPs) and by
  structured logging of which filter policies matched (or didn't) per call in
  non-production log levels.

Follow-up
~~~~~~~~~

- Decide, once real policy sets exist, whether ``access_policies`` needs an explicit
  deny/priority mechanism (today: purely additive) — deferred until a concrete case
  requires it, consistent with the Phase 1 scope in
  :ref:`architecture-design-history-phase-1-scope`.

Validation
----------

Unit tests in ``router`` for the two entry points independently (``Authorize``,
``FieldsToRemove``) plus one integration test exercising the full pipeline in
:ref:`architecture-data-request-lifecycle`: a claim set that is granted access to a tool
and also matches a filter policy for that same tool, asserting both the call succeeds
and the specified fields are absent from the response.

References
----------

- :doc:`/architecture/data` — ``AccessPolicy`` / ``FilterPolicy`` shapes and config examples.
- :doc:`ADR-0002 </architecture/decisions/0002-jsonpath-regexp-claim-rule-engine>` — the shared ``ClaimRule``/``ClaimMatcher``.
- :ref:`architecture-overview-pipeline-per-request` — the pipeline this ADR implements.
