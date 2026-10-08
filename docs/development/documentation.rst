Writing Documentation
=====================

This site is built with `Sphinx <https://www.sphinx-doc.org>`_ from
reStructuredText sources in ``docs/``
(:doc:`ADR-0022 </architecture/decisions/0022-sphinx-and-restructuredtext-for-project-documentation>`).
Documentation is part of a change, not a follow-up: a pull request that
changes behaviour updates the pages that describe it.

Tooling
-------

``docs/`` is a self-contained Python project managed with
`uv <https://docs.astral.sh/uv/>`_: ``pyproject.toml`` declares Sphinx and its
extensions, ``uv.lock`` pins them. ``uv`` creates the environment on first use.

.. code-block:: bash

   make docs          # build into docs/_build/html
   make docs-serve    # live preview at http://127.0.0.1:8000
   make docs-check    # strict build (every warning is an error) + link check

``make docs-check`` must pass; CI runs it on every pull request.

To add or upgrade a Sphinx extension:

.. code-block:: bash

   cd docs
   uv add sphinx-something    # updates pyproject.toml and uv.lock

Where things go
---------------

A page's path in ``docs/`` is its path on the site. Choose the section by
what the reader is trying to do:

.. list-table::
   :header-rows: 1
   :widths: 25 75

   * - Section
     - For
   * - ``getting-started/``
     - First contact: run it, install it, first configuration.
   * - ``concepts/``
     - Understanding: the model and why it works that way.
   * - ``use-cases/``
     - Solving a problem end to end.
   * - ``how-to/``
     - One operational task, step by step.
   * - ``reference/``
     - Looking something up: every key, endpoint, flag — exact and complete.
   * - ``features/``
     - One operator-visible feature in depth.
   * - ``architecture/``
     - How it is built; decision records in ``architecture/decisions/``.
   * - ``development/``
     - Working on mcplake itself.

Add every new page to its section's ``index.rst`` toctree — a page in no
toctree fails the build.

Markup conventions
------------------

Headings, in this order:

.. code-block:: rst

   Page Title
   ==========

   Section
   -------

   Subsection
   ~~~~~~~~~~

Links — always Sphinx roles, never relative file links, so the build checks
them:

.. code-block:: rst

   :doc:`/concepts/claim-rules`                   another page (uses its title)
   :doc:`claim rules </concepts/claim-rules>`     another page, custom text
   :ref:`reference-configuration-4-mcp-servers`   a labelled section
   :repo:`compose.yaml`                           a file in the repository
   :issue:`211`                                   a GitHub issue

To link to a section, give it an explicit label, prefixed with the page's
path:

.. code-block:: rst

   .. _how-to-use-postgresql-moving-from-sqlite:

   Moving from SQLite
   ------------------

Code — ``.. code-block::`` with a language (``toml``, ``bash``, ``json``,
``python``, ``text``). Prefer ``.. literalinclude::`` of a real file over a
copy that can drift.

Also available: admonitions (``.. note::``, ``.. tip::``, ``.. warning::``,
``.. important::``), diagrams (``.. mermaid::``), and ``sphinx-design``
grids, cards and tabs.

.. note::

   Inline code cannot be nested inside bold or italics in reStructuredText.
   The first line below renders with literal asterisks; write the second:

   .. code-block:: rst

      **the ``foo`` key**
      the ``foo`` key

Accuracy
--------

- Run every command and configuration example you write, against the
  current code. The quickest way is a local gateway with the demo's
  ``employee_directory.py`` MCP server.
- When behaviour changes, update every page that describes it: typically the
  reference page, the concept or feature page, and any tutorial whose steps
  change.
- Say what is not supported or not yet implemented rather than leaving it
  out.

Publishing
----------

The site is published to GitHub Pages in two versions: **stable**, built from
``main``, and **latest**, built from ``develop``. Merging to either branch
publishes; there is nothing to do by hand.
