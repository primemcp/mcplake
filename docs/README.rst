mcplake documentation
=====================

The sources of the mcplake documentation site. ``docs/`` is the Sphinx
source root and a self-contained Python project managed with
`uv <https://docs.astral.sh/uv/>`_ (see ADR-0022 in
``architecture/decisions/``).

From the repository root::

    make docs          # build into docs/_build/html
    make docs-serve    # live preview on http://127.0.0.1:8000
    make docs-check    # strict build + link check, as CI runs it

Or from this directory: ``make html``, ``make serve``, ``make check``.
