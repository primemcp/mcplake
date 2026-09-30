ADR-0015: Response Filtering Applies to the Tool Payload, Not the Transport Envelope
====================================================================================

:Status: Accepted
:Date: 2026-09-22

Context
-------

:doc:`ADR-0004 </architecture/decisions/0004-unified-policy-engine-for-access-and-filtering>` established
that one claim-rule engine answers both "may this caller make this call" and
"which fields must be removed from the answer". It did not say *which document*
those field paths are evaluated against, and the implementation picked the wrong
one.

A security audit of ``develop`` (2026-09-22) found that field redaction — the
capability this product exists to provide — silently did nothing for the
configuration the documentation tells operators to write:

- ``mcp.Client.CallTool`` stores ``json.Marshal(*sdk.CallToolResult)`` in
  ``ToolResponse.Raw``, i.e. the whole JSON-RPC envelope, and the data plane
  handed exactly that to ``filter.Strip``.
- Every ``drop_fields`` example in the repository is authored against the tool's
  own record — ``docs/CONFIG.md``, ``config.example.toml`` and ``docs/architecture/data.md``
  all show ``["$.hashed_password", "$.api_key", "$.internal_id"]``.
- Those paths resolve to nothing inside an envelope, and a non-resolving path is
  a deliberate no-op rather than an error, so the gateway returned the complete
  unredacted upstream body with ``200 OK``, no error and no log line.

Correcting the paths would not have been enough. An MCP result can carry the
same payload twice. When a typed tool handler leaves ``Content`` unset, the Go SDK
adds a fallback text block containing the payload serialized as a JSON **string**
("return the serialized JSON in a TextContent block, as the spec suggests" —
``go-sdk/mcp/server.go``). JSONPath cannot traverse into a string, so an
envelope-relative ``$.structuredContent.hashed_password`` removes one copy and
leaves an identical one in ``content[0].text``. The repository's own end-to-end
test demonstrated this: it stripped from ``structuredContent``, asserted only on
``structuredContent``, and green-lit a response that still contained the field.

Constraints on the fix:

- **The operator-facing contract must not get harder.** ``drop_fields`` naming
  fields of the tool's record is the only form that is guessable, and it is what
  four documents already promise.
- **Every representation of the record must be covered**, present and future —
  a mechanism that only knows about today's two places will rot.
- **A filter that cannot be enforced must not fail open.** Some tool results are
  free-form text; field paths are meaningless against them.
- No new dependency, no change to the policy engine, no per-tool configuration.

Decision
--------

**``drop_fields`` are evaluated against the tool's payload. The gateway unwraps
the transport envelope and applies them to every representation of that payload
it contains.**

A new ``filter.StripToolResult(response, fields) (filtered, removed, error)``
replaces ``filter.Strip`` in the data-plane pipeline:

1. Decode the envelope, preserving every key it does not understand (``isError``,
   ``_meta``, anything a future protocol revision adds).
2. If ``structuredContent`` is present, strip it as a document in its own right.
3. For each ``content`` block, dispatch on its ``type``: a ``text`` block's own
   text is decoded, stripped and re-encoded; a ``resource`` block's
   ``resource.text`` gets the identical treatment (``resource.blob``, its
   binary form, is left alone — nothing to filter there). Anything else
   (image, audio, a resource *link*, which carries a URI rather than
   inline content) has no JSON fields and passes through.
4. Return the re-marshaled envelope and the number of locations actually
   removed.

**Fail closed.** If a filter policy applies to the call and a text block is not
a JSON document, its fields cannot be applied. ``StripToolResult`` returns
``filter.ErrUnenforceable`` and the data plane answers
``502 filter_unenforceable`` instead of a response nobody checked. With no fields
in force there is nothing to enforce, and such a response passes through
untouched — a text-only tool keeps working until someone points a filter at it.

**Surface dead filters.** The removal count lets the gateway log a ``WARN`` when a
policy matched the call and then removed nothing — almost always a path that no
longer matches the tool's shape after a rename. That case was previously
indistinguishable from success.

``filter.Strip`` remains as the single-document primitive ``StripToolResult`` is
built on, and keeps its "a path that does not resolve is a no-op" contract:
``drop_fields`` are authored once against a tool's general shape, and an optional
field being absent from one response is not an error.

Alternatives Considered
-----------------------

Alternative A: Document envelope-relative paths and leave the code alone
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Zero code change; the mechanism already works for ``structuredContent``.

Disadvantages:

- Does not fix the actual leak. The duplicate in ``content[*].text`` survives any
  JSONPath expression, because it is a string.
- Pushes a protocol implementation detail into every operator's config, and one
  that changes with the SDK's fallback behaviour.
- Contradicts four existing documents rather than fixing the code that
  contradicts them.

Alternative B: Filter inside ``mcp.Client``, before the envelope is built
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- The record is still a structured value there; no unwrap needed.

Disadvantages:

- Inverts the dependency the architecture rests on: ``mcp`` is the transport
  adapter and must not know about policies. The filter decision needs claims,
  which belong to the request pipeline, not the client.
- ``mcp.Client`` is shared by every caller of an MCP; a per-request concern has no
  business there.

Alternative C: Strip the duplicate text block whenever a filter applies
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Trivially safe — no filtered copy can leak if the copy is gone.

Disadvantages:

- Breaks MCP clients that read ``content`` rather than ``structuredContent``, which
  the spec explicitly supports and which pre-SEP-2106 clients require.
- Turns "redact three fields" into "lose the entire human-readable result",
  which is a much larger behaviour change than the operator asked for.

Alternative D: Fail closed on *any* content the filter cannot parse, including non-text blocks
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Maximally conservative.

Disadvantages:

- An image or audio block has no JSON fields to leak, so refusing the call
  protects nothing and breaks legitimate multimodal tools.

This alternative is about the genuinely opaque block types. An embedded
resource (``type: "resource"``) is not one of them and must not be lumped in
with image/audio by analogy: ``resource.text`` has the identical shape as a
top-level text block — a string that is commonly JSON — and is filtered the
same way (``resource.blob``, the binary form, has no JSON to leak and is left
alone, same reasoning as image/audio). An earlier draft of this decision
missed that distinction and treated every non-``"text"`` block as opaque,
which silently left ``resource.text`` unfiltered; a follow-up review caught
it before merge.

Decision Criteria
-----------------

1. Does the documented configuration actually redact? (A, no.)
2. Is every copy of the payload covered? (A, no; C, by deletion.)
3. Does the fix respect the module boundaries? (B, no.)
4. Is an unenforceable filter impossible to mistake for a working one?
5. Cost to operators: no config rewrite, no new concepts.

Only the chosen design satisfies all five.

Rationale
---------

The bug was not that the paths were wrong; it was that the filter was pointed at
the transport rather than at the data. Fixing the layer the filter operates on
keeps the operator's mental model ("name the fields your callers must not see")
exactly as documented, and makes correctness independent of how many copies the
protocol decides to carry.

Failing closed is the only defensible answer to "a filter applied and could not
be enforced". A gateway whose entire purpose is withholding fields must not
return a body it was unable to inspect; an operator would rather see a 502 they
can act on than a silent leak they cannot see.

Consequences
------------

Positive
~~~~~~~~

- Documented ``drop_fields`` work, against real MCP servers, for the first time.
- Both copies of the payload are covered, so there is no path form that leaks.
- A filter that matches but removes nothing is now visible in the logs.
- Text-only tools are no longer silently unfilterable: they either filter or the
  call fails.

Negative
~~~~~~~~

- **Breaking change for anyone who worked out the envelope-relative form.** A
  policy written as ``$.structuredContent.secret`` now matches nothing — it is
  interpreted against the record, where there is no ``structuredContent`` key. The
  fix is to drop the prefix. Only the repository's own e2e test was written that
  way; no documentation ever described it.
- A tool that returns free-form text and is targeted by a filter policy now
  fails the call with ``502 filter_unenforceable`` where it previously returned
  (unfiltered) data. This is intended, and the log line names the mcp and tool.
- Slightly more work per filtered response: one extra decode/encode of the text
  block. Only when a filter actually applies.

Risks
~~~~~

- An MCP that returns a JSON *scalar* or array as its text content is filtered
  as a document; field paths simply do not match, which the removal-count WARN
  will surface rather than hide.
- Future protocol revisions could add a third place a payload can appear.
  Mitigated by preserving unknown envelope keys verbatim and by the WARN, but a
  new location would need explicit support.

Follow-up
~~~~~~~~~

- Validate ``drop_fields`` against the tool's cached ``OutputSchema`` at policy
  write time, so an unmatchable path is rejected when it is created rather than
  noticed in a log line later (the schema is already in ``cache.ToolSchema``).
- Consider promoting the "matched but removed nothing" WARN to a metric once
  observability lands (Phase 3).

Validation
----------

- ``filter``: ``StripToolResult`` unit tests covering both copies, structured-only
  and text-only results, an embedded resource's text (and its untouched binary
  blob), non-text content, unknown envelope keys preserved, large integers and
  HTML-significant characters surviving byte-for-byte, the unmatched-path
  no-op, and ``ErrUnenforceable`` on free-form text (including inside a
  resource).
- ``gateway/internal``: pipeline tests asserting the filtered field is absent from
  the **whole** response body, that an unenforceable filter yields
  ``502 filter_unenforceable`` without echoing the payload, and that free-form
  text passes through when no filter applies.
- ``cmd/gateway/app``: the existing enable/disable end-to-end test now drives a
  real SDK fixture MCP with record-relative ``drop_fields`` and asserts the
  stripped value is absent from every copy in the envelope.

References
----------

- :doc:`ADR-0004 </architecture/decisions/0004-unified-policy-engine-for-access-and-filtering>` — the
  engine whose ``DropFields`` this ADR gives a precise meaning to.
- :doc:`ADR-0003 </architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery>` — the cached
  ``OutputSchema`` the follow-up would validate against.
- :repo:`CONFIG.md <docs/CONFIG.md>` — ``drop_fields`` operator documentation.
- :repo:`api/data-plane.md <docs/api/data-plane.md>` — the ``filter_unenforceable``
  response.
- MCP specification, "Structured content":
  https://modelcontextprotocol.io/specification/2025-06-18/server/tools#structured-content
