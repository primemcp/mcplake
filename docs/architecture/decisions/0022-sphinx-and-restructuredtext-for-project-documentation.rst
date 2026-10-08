ADR-0022: Sphinx and reStructuredText for Project Documentation
===============================================================

:Status: Accepted
:Date: 2026-09-30

Context
-------

The project's documentation is a set of Markdown files under ``docs/``:
architecture pages, twenty-one decision records, API and configuration
references, feature pages and a demo walkthrough. They are readable only by
browsing the repository on GitHub. That was enough while the audience was the
people writing the code; it is not enough for the audience the project is
built for.

- **Operators evaluating the gateway have no entry point.** There is no
  getting-started path, no explanation of the concepts in the order a newcomer
  needs them, and no material that starts from a problem ("keep one team
  read-only", "hide salary fields") rather than from a component.
- **Nothing ties the pages together.** Navigation is the directory listing.
  There is no search, no table of contents across pages, and a broken link
  between pages is found only when someone clicks it.
- **The docs are not versioned against releases.** ``main`` is the release
  branch and ``develop`` carries unreleased work, but a reader cannot tell
  which behaviour a page describes.
- **The site has to be published somewhere.** The README already anticipates
  a GitHub Pages landing page (:issue:`209`).

Requirements:

- A static site that can be hosted on GitHub Pages, with navigation, full-text
  search and a light/dark theme.
- A build that fails on broken cross-references, so the docs cannot silently
  rot, and that runs in CI.
- Two published versions: *stable* from ``main`` and *latest* from
  ``develop``.
- A markup rich enough for reference material: admonitions, cross-page
  references to sections, tables, tabs (Docker vs Podman), diagrams, and
  rendering the control-plane OpenAPI spec (ADR-0007) rather than restating
  it.
- One source of truth. The same content must not be maintained in two
  formats.

Decision
--------

**Build the documentation site with Sphinx from reStructuredText sources, and
make RST the only format for persistent project documentation.**

- ``docs/`` is the Sphinx source root and a self-contained Python project:
  ``docs/pyproject.toml`` declares Sphinx and its extensions, ``docs/uv.lock``
  pins them, and `uv <https://docs.astral.sh/uv/>`_ runs every build. The
  Go workspace is unaffected.
- Pages live at the same repository path they are published under, so
  ``docs/architecture/decisions/`` stays the home of decision records and
  existing links need only an extension change.
- Existing Markdown documents are converted to RST and deleted. Internal
  working notes that are not documentation (``docs/superpowers/``,
  ``docs/SCAFFOLDING.md``) stay Markdown and are excluded from the site.
- Theme: `Furo <https://pradyunsg.me/furo/>`_. Extensions:
  ``sphinx-copybutton``, ``sphinx-design`` (cards, tabs),
  ``sphinxcontrib-mermaid`` (diagrams), ``autosectionlabel`` (section
  references), ``extlinks`` (issue and source links).
- The build is strict: ``make docs-check`` runs ``sphinx-build -W`` (every
  warning is an error) and ``linkcheck``. CI runs it on pull requests.
- GitHub Pages serves ``/stable/`` built from ``main`` and ``/latest/`` built
  from ``develop``; both are built and deployed together by one workflow.

Alternatives Considered
-----------------------

Sphinx with MyST (Markdown)
   Keeps the existing files as they are. Rejected because it leaves two markup
   dialects in one site, and MyST's directive syntax is a Markdown extension
   anyway, so contributors still learn Sphinx semantics.

MkDocs (Material)
   A strong Markdown-first generator with good versioning support via
   ``mike``. Rejected in favour of Sphinx's checked cross-references,
   ``autosectionlabel`` and the reStructuredText directive set.

Hugo or another Go static-site generator
   Keeps the toolchain in Go. Rejected: these are website builders, and
   reference-grade documentation features (checked references, admonitions,
   API rendering) would have to be assembled from themes and shortcodes.

Keep Markdown on GitHub only
   No cost. Rejected because it meets none of the requirements above.

Decision Criteria
-----------------

- Checked cross-references and a strict build that can gate CI.
- Quality of reference-documentation features (admonitions, field lists,
  includes of real files, API rendering).
- Static output hostable on GitHub Pages, with two versions side by side.
- Single source format.
- Toolchain cost for contributors who work mostly in Go.

Rationale
---------

Sphinx is the generator whose core job is reference-grade technical
documentation: its cross-reference system is the reason a broken link can
fail the build, and reStructuredText's directive set covers every construct
listed in the requirements without extensions to the markup itself. The
toolchain cost is real but contained: it lives entirely in ``docs/``, is
pinned by a lock file, and ``uv`` reduces it to one binary. Converting the
existing Markdown once is cheaper than maintaining a mixed-format site
indefinitely.

Consequences
------------

Positive:

- Cross-references between pages and sections are checked at build time.
- Readers get search, navigation and a version they can match to a release.
- RST directives cover reference material (field lists, admonitions,
  ``literalinclude`` of real config files) without HTML escapes.

Negative:

- Contributors need ``uv`` to preview docs, and a Python toolchain now sits
  next to the Go one. It is confined to ``docs/`` and not needed to build or
  test the gateway.
- RST is less familiar than Markdown, and GitHub renders it less completely
  (Sphinx-only directives show as raw text when browsing the repository).
- Converting the existing Markdown is a one-off cost, and links into the old
  ``.md`` paths from outside the repository break.
