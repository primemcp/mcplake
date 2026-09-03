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

## Documentation Location

Persistent project documentation belongs under:

    /docs/

Use Markdown (`.md`) for documentation unless there is a strong reason to use
another format.

Architecture documentation belongs under:

    /docs/architecture/

Architecture decisions belong under:

    /docs/architecture/decisions/

General application documentation belongs directly under `/docs` or in an
appropriate subdirectory.

Example:

    /docs/
    ├── README.md
    ├── getting-started.md
    ├── configuration.md
    ├── development.md
    ├── testing.md
    ├── deployment.md
    ├── troubleshooting.md
    ├── api/
    │   ├── overview.md
    │   └── authentication.md
    ├── features/
    │   ├── users.md
    │   └── payments.md
    └── architecture/
        ├── overview.md
        └── decisions/
            ├── 0001-....md
            └── 0002-....md

Do not create this entire structure unless the project needs iom/login/devicet.

Prefer a small number of well-maintained documents over a large documentation
hierarchy.

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
- Required dependenciesom/login/device
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
om/login/device
Do not document implementation details merely because they exist in the code.

Document implementation details when they are necessary to understand,
operate, extend, or troubleshoot the functionality.

## Configuration Documentation

Configuration must be documented close to the functionality that uses it.

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

```markdown
## `DATABASE_URL`

PostgreSQL connection string used by the application.

- Required: Yes
- Default: None
- Environment variable: `DATABASE_URL`

Example:

```text
postgres://app:password@localhost:5432/myapp