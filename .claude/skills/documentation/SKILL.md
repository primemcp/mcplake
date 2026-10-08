---
name: documentation
description: >
  Create and maintain project documentation under /docs. Use when implementing
  functionality, changing application behavior, configuration, APIs, commands,
  workflows, testing procedures, deployment, troubleshooting, or other behavior
  that future developers or users need to understand. Documentation must be
  created and updated as part of development, not postponed until the end.
---

# Documentation

Documentation is part of the implementation, not a separate phase after
implementation.

When developing or changing functionality, update the relevant documentation
in `/docs` during the same task.

The project should remain understandable and usable by someone who did not
implement the feature.

## Format and Tooling

Project documentation is a **Sphinx** site written in **reStructuredText**
(`.rst`), published to GitHub Pages
([ADR-0022](../../../docs/architecture/decisions/0022-sphinx-and-restructuredtext-for-project-documentation.rst)).

- `docs/` is the Sphinx source root **and** a self-contained Python project
  (`docs/pyproject.toml`, `docs/uv.lock`, `docs/conf.py`), managed with `uv`.
  It is not a Go module.
- A page's repository path is its site path: `docs/how-to/foo.rst` is
  published as `how-to/foo.html`.
- Write new documentation as `.rst`. Do **not** add Markdown pages: Sphinx
  only reads `.rst`, so a `.md` file under `docs/` is invisible on the site.
  The only Markdown left under `docs/` is internal working notes
  (`docs/superpowers/`, `docs/SCAFFOLDING.md`), excluded from the site.
- Top-level `README.md`, `CONTRIBUTING.md` and Go code comments stay as they
  are; when they point at documentation, point at the `.rst` file.

Build and check:

```bash
make docs          # build into docs/_build/html
make docs-serve    # live-reloading preview on http://127.0.0.1:8000
make docs-check    # strict build (-W: every warning is an error) + linkcheck
```

`make docs-check` must pass before committing documentation changes. CI runs
it on every pull request.

## Documentation Location

    /docs/
    ├── index.rst              # landing page, top-level toctrees
    ├── conf.py, pyproject.toml, uv.lock, Makefile
    ├── getting-started/       # first run, first config
    ├── concepts/              # how the gateway thinks: planes, policies, rules
    ├── use-cases/             # task-driven tutorials
    ├── how-to/                # focused operational recipes
    ├── reference/             # configuration, APIs, CLI — exhaustive and exact
    ├── features/              # one page per operator-visible feature
    ├── development/           # contributor workflow
    └── architecture/
        ├── index.rst
        ├── overview.rst, components.rst, data.rst, security.rst
        └── decisions/         # ADRs (see the architecture skill)

Put a page in the section matching what the reader is trying to do: learn
(getting started, use cases), accomplish a task (how-to), look something up
(reference), or understand (concepts, architecture).

Every new page must be added to a `toctree` (usually the section's
`index.rst`); an orphan page is a build warning and fails `docs-check`.

Prefer a small number of well-maintained pages over a large hierarchy.

## Writing reStructuredText

Headings — use this adornment order consistently:

```rst
Page Title
==========

Section
-------

Subsection
~~~~~~~~~~
```

Cross-references — always use Sphinx roles, never raw relative links, so
the build can verify them:

```rst
:doc:`/architecture/overview`                  page, shows its title
:doc:`the overview </architecture/overview>`   page, custom text
:ref:`reference-config-server`                 labelled section
:repo:`compose.yaml`                           file in the repository (GitHub link)
:issue:`211`                                   GitHub issue
```

To make a section referenceable, put an explicit label above it. Labels are
global, so prefix them with the page path:

```rst
.. _reference-config-server:

``[server]``
------------
```

Code and config — use `code-block` with a language, and prefer including
real files over copying them, so examples cannot drift:

```rst
.. code-block:: toml

   [server]
   data_plane_addr = ":8080"

.. literalinclude:: /../config.example.toml
   :language: toml
   :lines: 1-10
```

Also available: admonitions (`.. note::`, `.. warning::`), diagrams
(`.. mermaid::`), and `sphinx-design` cards and tabs (`.. tab-set::` for
Docker vs Podman style alternatives).

## Documentation-First During Development

Documentation must be updated as functionality is developed.

Do NOT follow this workflow:

    implement everything
        ↓
    finish application
        ↓
    write documentation

Prefer:

    understand requirement
        ↓
    implement functionality
        ↓
    document functionality
        ↓
    test implementation and documentation examples
        ↓
    continue development

For a feature that changes during development, update its documentation in
the same task.

A feature is not considered complete if its required documentation is missing.

## What Should Be Documented

Document functionality that another developer, operator, or user would need
to understand or use.

Depending on the feature, document:

- What the feature does
- Why it exists
- How to enable it
- How to configure it
- Configuration options
- Environment variables
- Required dependencies
- Default values
- CLI commands
- API endpoints
- Request/response examples
- Authentication requirements
- Authorization requirements
- User workflows
- Examples
- Expected behavior
- Error behavior
- Failure modes
- Testing instructions
- Local development instructions
- Deployment requirements
- Operational considerations
- Troubleshooting
- Migration/upgrade instructions
- Limitations
- Security considerations
- Performance considerations when relevant

Do not document implementation details merely because they exist in the code.

Document implementation details when they are necessary to understand,
operate, extend, or troubleshoot the functionality.

A behavior change usually touches more than one section: the reference page
(exact keys and endpoints), the feature or concept page (what and why), and
any use case or how-to whose steps change. Update all of them.

## Configuration Documentation

Configuration must be documented close to the functionality that uses it,
and every key must appear in the configuration reference.

For every meaningful configuration option, document where applicable:

- Name
- Purpose
- Type
- Required/optional
- Default value
- Valid values
- Example
- Environment variable name
- Security sensitivity
- When the application reads it
- Whether a restart is required
- Interactions with other configuration

Example:

```rst
.. _reference-config-persistence-dsn:

``dsn``
~~~~~~~

PostgreSQL connection string used for control-plane persistence.

:Type: string
:Required: when ``driver = "postgres"``
:Default: none
:Sensitive: yes — contains credentials

.. code-block:: toml

   [persistence]
   driver = "postgres"
   dsn = "postgres://mcplake:secret@db:5432/mcplake"
```

## Verify Examples

Every command, request and config snippet in the docs must work against the
current code. When you change behavior, re-run the examples that describe
it; when you write a new example, run it. Do not document behavior you have
not verified.
