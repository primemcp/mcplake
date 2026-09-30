Contributing
============

Contributions are welcome, from bug reports to features. mcplake is licensed
under the Apache License 2.0; by contributing you agree your contribution is
licensed the same way.

From issue to merge
-------------------

Work is tracked in GitHub issues and follows GitFlow:

1. **Start from an issue.** Open or pick one describing the change. Larger
   efforts are organized as epics with task issues under them.
2. **Branch from** ``develop``, named after the issue:
   ``feature/<issue-number>_<short_title>`` — for example
   ``feature/212_docs_sphinx_scaffold``.
3. **Implement, with tests and documentation** in the same branch
   (:doc:`documentation`).
4. **Check before pushing:** ``make check``, plus ``make ui-test`` and
   ``make docs-check`` if you touched the UI or the docs.
5. **Commit messages** start with the issue number and say what the change
   does: ``#205 Route the control plane on the escaped path``.
6. **Open a pull request into** ``develop`` that references the issue,
   explains the change and the decisions behind it, and lists how it was
   tested and any known limitations.
7. **Review.** Every pull request is reviewed by a maintainer before it is
   merged.

``main`` is the release branch: it changes only through releases from
``develop``, never through feature pull requests.

Reporting bugs
--------------

Include the gateway version (commit), the relevant configuration (with
secrets removed), the request, what you expected and what happened, and the
gateway's log lines around it.

Security issues
---------------

Do not open a public issue for a vulnerability. Contact the maintainers
privately through GitHub's security advisory feature on the repository.
