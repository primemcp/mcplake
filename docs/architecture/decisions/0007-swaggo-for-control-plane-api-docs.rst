ADR-0007: swaggo (swag + gin-swagger) for Control-Plane API Documentation
=========================================================================

:Status: Accepted
:Date: 2026-09-04

Context
-------

:doc:`ADR-0005 </architecture/decisions/0005-use-gin-for-control-plane-api>` defines a CRUD admin API
(``/admin/mcps``, ``/admin/access-policies``, ``/admin/filter-policies``) for registering
MCPs and managing access/filter policies. As that surface grows (this milestone adds
~10 endpoints across three resources, per the `Epic #4 task list <https://github.com/primemcp/mcplake/issues/4>`__),
operators and future Admin/Graph UI developers need a discoverable, accurate
reference for request/response shapes without reading Go source.

Requirements:

- The spec must stay honest — a hand-maintained doc that drifts from the actual
  Gin routes is worse than no doc, since operators will trust and act on it.
- Low overhead to keep current: whoever adds or changes an admin endpoint should
  update the spec as a natural part of that change, not as a separate documentation
  task that's easy to skip.
- An interactive "try it" UI is valuable for operators exploring the admin API
  (registering an MCP, authoring a policy) without writing ``curl`` by hand.
- Consistent with the project's stated bias toward widely adopted, easy tooling
  (the same reasoning behind choosing Gin itself in ADR-0005).

Decision
--------

Use ```swaggo/swag`` <https://github.com/swaggo/swag>`__ to generate an OpenAPI
(Swagger 2.0) spec from Go doc-comment annotations placed directly above each Gin
handler, and serve it with ```gin-swagger`` <https://github.com/swaggo/gin-swagger>`__ at
``/admin/swagger/index.html`` on the control-plane port — the same trust boundary as
the rest of ``/admin/*`` (per ADR-0005's follow-up: not exposed beyond a trusted
network by default).

``swag init`` runs as a build/CI step, regenerating ``docs/swagger.json``/``docs.go`` from
the handler annotations. CI runs ``swag init`` and fails the build on a diff against
the committed output, so an endpoint change without a matching annotation update is
caught before merge rather than silently drifting.

Alternatives Considered
-----------------------

Alternative A: Hand-written OpenAPI YAML (spec-first)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Full control over the spec's shape; not constrained by what a code-first
  generator can express; language-agnostic if the stack ever changes.

Disadvantages:

- Nothing enforces that the YAML matches the actual Gin routes/handlers — keeping it
  current is a separate, easily-skipped step for every future endpoint change,
  which is exactly the drift risk called out in the Context.

Alternative B: swaggo (``swag`` + ``gin-swagger``) (chosen)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- The de facto standard pairing for Gin specifically — most widely adopted option
  for this exact stack, consistent with the "widely adopted, easy" criterion already
  used to choose Gin.
- Annotations live next to the handler they describe, so a reviewer sees the route
  and its doc-comment in the same diff hunk.
- ``gin-swagger`` gives an interactive UI for free once the spec exists.

Disadvantages:

- Still hand-maintained annotations, not free automatic sync — mitigated by the CI
  diff-check described in the Decision, which catches missing updates at merge time
  rather than relying on developer discipline alone.
- Adds two dependencies (``swag``, ``gin-swagger``) plus a generated-file step to the
  build.

Alternative C: ``oapi-codegen`` or another spec-first Go codegen tool
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Can generate server stubs and client code from a single source-of-truth YAML,
  useful if multiple languages/clients need generated bindings.

Disadvantages:

- Primarily built around ``net/http``/``chi``-style routing patterns; weaker Gin
  integration than swaggo's purpose-built ``gin-swagger`` middleware.
- Spec-first workflow reintroduces Alternative A's drift risk between the YAML and
  the actual Gin handler implementation unless paired with additional generation
  discipline the project doesn't otherwise need yet.

Decision Criteria
-----------------

- Fit with Gin specifically (this is a Gin-only surface — the fasthttp data plane is
  explicitly out of scope, per ADR-0005).
- Drift resistance between spec and implementation.
- Ecosystem maturity / ease of adoption, consistent with prior tooling choices.
- Value of an interactive exploration UI for operators.

Rationale
---------

swaggo is the standard choice for documenting a Gin API, keeps the spec next to the
code that implements it, and — with a CI diff-check — closes most of the drift gap
that a hand-written spec would leave open. It requires no new architectural pattern
beyond what ADR-0005 already introduced (Gin), just a documentation convention on
top of it.

Consequences
------------

Positive
~~~~~~~~

- Every admin endpoint gets a discoverable, testable interactive reference with
  minimal added process (a doc-comment at the point of change).
- CI catches undocumented/changed endpoints before merge.

Negative
~~~~~~~~

- Two more dependencies and a generated-file (``docs/swagger.json``, ``docs.go``) that
  must be committed and kept in sync via the CI check.
- Annotation comments add some verbosity to handler files.

Risks
~~~~~

- The CI diff-check only catches drift if contributors actually run ``swag init``
  before pushing; a pre-commit hook (follow-up) would close this gap further than CI
  alone, which only catches it at PR time.
- Swagger UI sits behind the same control-plane trust boundary as the rest of
  ``/admin/*`` (ADR-0005) — if that boundary is ever misconfigured to be
  publicly reachable, the spec itself becomes a map of the admin surface for an
  attacker. This is the existing ADR-0005 risk, not a new one, but worth restating
  since a docs UI makes the surface easier to explore.

Follow-up
~~~~~~~~~

- Add a pre-commit or ``make check`` step that runs ``swag init`` and fails on diff,
  not just CI, to shorten the feedback loop.
- Once the admin authN/authZ story from ADR-0005's follow-up is designed, ensure the
  Swagger UI route is covered by it like any other ``/admin/*`` route.

Validation
----------

CI check: run ``swag init``, assert no diff against committed ``docs/swagger.json``.
Manual check: ``/admin/swagger/index.html`` renders and its "try it" flow can
successfully exercise ``POST /admin/mcps`` against a running gateway.

References
----------

- :doc:`ADR-0005 </architecture/decisions/0005-use-gin-for-control-plane-api>` — the API surface this documents.
- :ref:`architecture-components-control-plane-api-gin`
- Epic: https://github.com/primemcp/mcplake/issues/4
