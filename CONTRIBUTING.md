# Contributing to mcplake

Contributions are welcome. The full contributor guide is part of the
documentation:

- [Contributing](docs/development/contributing.rst) — issues, branches, pull requests and review
- [Repository and workspace](docs/development/workspace.rst) — modules, layout and `make` targets
- [Testing](docs/development/testing.rst)
- [Admin web UI development](docs/development/admin-ui.rst)
- [Writing documentation](docs/development/documentation.rst)

## In short

1. Start from a GitHub issue.
2. Branch from `develop` as `feature/<issue-number>_<short_title>`.
3. Include tests and documentation with the change.
4. Run `make check` (and `make ui-test` / `make docs-check` if you touched the UI or docs).
5. Open a pull request into `develop` that references the issue.

### Prerequisites

- Go 1.27.1 or later
- [Bun](https://bun.sh) for the admin web UI
- [uv](https://docs.astral.sh/uv/) for the documentation

The dev container in `.devcontainer/` has all of them.

## License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0.
