ADR-0012: TOML Configuration File Format
========================================

:Status: Accepted
:Date: 2026-09-10

Context
-------

The gateway reads a single configuration file at startup (``--config``, default
``config.yaml``) covering the two listen addresses, OIDC/JWKS settings, admin
authentication rules, the control-plane MCP server toggle, persistence, and the
seed lists of MCPs / access policies / filter policies. It is parsed by the
``config`` module with ``go.yaml.in/yaml/v3`` into a ``config.Config``, then
``Config.Validate()`` runs.

The file is edited by operators. Several YAML *format* properties are a poor fit
for that:

- **Significant whitespace.** A misindented key is silently reparented or
  dropped rather than rejected — a real hazard for a security config where a
  dropped ``admin_auth`` block means "unauthenticated".
- **Implicit type coercion.** Unquoted scalars are type-guessed: ``no``/``off``/``yes``
  become booleans (the "Norway problem": ``country: NO``), version-like strings
  become numbers, leading-zero strings become octal or are truncated.
- **Anchors, aliases, and merge keys** (``&``, ``*``, ``<<``) add a macro layer with
  no benefit for a config this size and real surprise potential.
- The ``config`` module already carries a custom ``OIDCConfig.UnmarshalYAML`` shim
  purely to decode a duration from a string, because ``time.Duration`` has no
  native YAML form.

The library itself is not the problem: ``go.yaml.in/yaml/v3`` is the maintained
continuation of ``gopkg.in/yaml.v3`` and is current. This is a decision about the
*format*.

Constraints:

- The config is small, hierarchical but shallow (a handful of tables, three
  arrays of tables), and hand-edited — not machine-generated, not templated.
- The project is pre-1.0; a one-time breaking change to the config file is
  acceptable if it is clean (no long dual-format deprecation window).
- Duration-valued fields (``oidc.jwks_cache_ttl``, and
  ``mcp.schema_refresh_interval`` added by
  :doc:`ADR-0013 </architecture/decisions/0013-periodic-mcp-schema-refresh>`) should decode from a
  human string like ``"1h"`` / ``"15m"``.

Decision
--------

Adopt **TOML** as the configuration file format, parsed with
**``github.com/BurntSushi/toml``**.

- ``--config`` default becomes ``config.toml``; ``config.example.yaml`` becomes
  ``config.example.toml``.
- ``config`` module: ``toml:"..."`` struct tags; ``config.Load`` reads the file and
  ``toml.Decode``\ s it, then runs ``Config.Validate()`` unchanged.
- Duration fields use a small ``config.Duration`` wrapper implementing
  ``encoding.TextUnmarshaler`` (``time.ParseDuration`` on the text). ``TextUnmarshaler``
  is honoured by ``BurntSushi/toml`` and by ``encoding/json``, so the wrapper is
  format-agnostic and the bespoke ``UnmarshalYAML`` shim is deleted.
- ``*bool`` "omitted vs explicit ``false``" semantics for every ``enabled`` field are
  preserved: BurntSushi leaves an absent key's pointer ``nil``, exactly as the
  YAML decoder did.
- No dual-format loading. The switch is atomic; ``config.yaml`` is no longer read.

Arrays of tables (``[[mcps]]``, ``[[access_policies]]``, and nested
``[[access_policies.match]]`` / ``[[access_policies.grants]]``) express the seed
lists.

Alternatives Considered
-----------------------

Alternative A: ``github.com/pelletier/go-toml/v2``
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Also stable and widely used (the Viper/Cobra ecosystem's TOML decoder);
  faster; slightly richer API (decoder options, strict mode, position info).

Disadvantages:

- Larger API surface and more active feature churn than needed for "decode a
  small file into a struct once at startup".
- No practical advantage over BurntSushi for this use; BurntSushi is the more
  conservative dependency.

Both are fine; BurntSushi is chosen for being the smaller, more static
dependency. Switching later is a one-line import change.

Alternative B: ``sigs.k8s.io/yaml``
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- YAML restricted to the JSON-compatible subset, decoded via ``encoding/json``
  (JSON struct tags, ``json.Unmarshaler`` hooks); no anchors/merge keys.

Disadvantages:

- Not a distinct format — the file is still YAML and still has significant
  whitespace and the scalar-coercion quirks that motivated this ADR; it only
  removes the macro features.
- Built on an internal fork of the older ``gopkg.in/yaml.v2``.
- Drops ``yaml.Unmarshaler`` support (goes YAML→JSON→struct), so the custom
  decoding would move to ``json.Unmarshaler`` — a lateral move, not a
  simplification.

Alternative C: Stay on YAML (``go.yaml.in/yaml/v3``)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Advantages:

- Zero migration; the library is maintained.

Disadvantages:

- Keeps every format hazard above for a hand-edited security config.

Alternative D: JSON
~~~~~~~~~~~~~~~~~~~

Advantages:

- In the standard library; unambiguous.

Disadvantages:

- No comments (operators annotate this file heavily); trailing-comma and
  quoting friction for hand editing; poorer diff readability for the array
  sections.

Alternative E: HCL
~~~~~~~~~~~~~~~~~~

Advantages:

- Comments, good ergonomics for nested blocks.

Disadvantages:

- Heavier dependency; a format most contributors know less well than TOML;
  overkill for a file with no expressions or interpolation needs.

Decision Criteria
-----------------

- **Safety for a hand-edited security config** — no silent misparse, no scalar
  coercion surprises.
- **Dependency conservatism** — the smallest, most static library that does the
  job.
- **Comment support** — operators annotate the file.
- **Simplicity of the ``config`` module** — fewer bespoke decoding hooks.
- **Contributor familiarity.**

Rationale
---------

The config is small, shallow, and hand-edited — the exact case TOML was
designed for, and the exact case where YAML's whitespace sensitivity and
implicit typing cost the most. ``BurntSushi/toml`` is a decade-stable, minimal
dependency. Moving the two duration fields to an ``encoding.TextUnmarshaler``
wrapper deletes existing bespoke code and is format-agnostic. The project being
pre-1.0 makes the one breaking file change cheap now and cheaper than carrying
YAML's hazards indefinitely.

Consequences
------------

Positive
~~~~~~~~

- A misindented or mistyped key is a load-time error, not a silent drop.
- No ``yes``/``no``/``NO``/octal coercion class of bugs.
- The ``OIDCConfig.UnmarshalYAML`` shim is removed; duration handling is one
  small shared type.
- ``config.example.toml`` diffs and reviews are more legible in the array
  sections.

Negative
~~~~~~~~

- Breaking change: every existing ``config.yaml`` must be rewritten as
  ``config.toml``. Mitigated by the pre-1.0 status, a rewritten
  ``config.example.toml``, and a ``docs/reference/configuration.rst`` rewrite; called out in the
  changelog / release notes.
- Deeply nested inline structures (a ``match`` rule list inside a policy inside
  the policies array) are more verbose in TOML's ``[[a.b.c]]`` form than in
  YAML's indentation. Acceptable for a file of this size.
- One new direct dependency (``github.com/BurntSushi/toml``); one removed
  (``go.yaml.in/yaml/v3``) from the ``config`` module.

Risks
~~~~~

- Operators copy-pasting old YAML snippets from tickets/blog posts into the new
  file. Mitigated by docs and by the loader rejecting YAML outright (it is not
  valid TOML).

Follow-up
~~~~~~~~~

- Update ``docs/reference/configuration.rst``, ``README.md``, ``docs/api/*``, and the config snippets
  in ADRs 0002/0004/0005/0006/0009/0010/0011 (historical decision text stays;
  only the illustrative snippet changes).
- :doc:`ADR-0013 </architecture/decisions/0013-periodic-mcp-schema-refresh>` authors its new key in TOML.

Validation
----------

- ``config``: the full ``config/*_test.go`` suite re-expressed with TOML fixtures —
  valid example load, malformed-input error, semantic-validation errors,
  ``enabled`` present/absent/false, duration parsing.
- ``cmd/gateway/app``: unchanged (its tests build ``config.Config`` struct literals,
  not files); a manual ``--config config.toml`` smoke of the built binary.

References
----------

- :doc:`ADR-0006 </architecture/decisions/0006-gorm-sqlite-postgres-persistence>` — config file as the seed
  mechanism for the persisted control-plane state.
- :doc:`ADR-0003 </architecture/decisions/0003-dynamic-mcp-registration-and-schema-discovery>` — the
  ``mcps`` seed list this file carries.
- :doc:`ADR-0013 </architecture/decisions/0013-periodic-mcp-schema-refresh>` — adds ``mcp.schema_refresh_interval``.
- :doc:`/reference/configuration` — the configuration reference (rewritten for TOML).
