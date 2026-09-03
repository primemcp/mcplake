---
name: project-management
description: >
  Manage project planning and execution through GitHub using the `gh` CLI.
  Use milestones, epics, and tasks to turn plans into tracked, actionable work.
  Always keep the human in the loop before creating tasks or issues.
---

# GitHub Project Management

Use GitHub as the source of truth for project planning and work tracking.

The workflow is:

1. Understand the current project state.
2. Read existing milestones and issues.
3. Define or update milestones.
4. Organize larger bodies of work as epics.
5. Break epics into actionable tasks.
6. Ask the human for approval before creating tasks.
7. Implement approved tasks.
8. Keep GitHub issues updated as work progresses.

Use the GitHub CLI (`gh`) for GitHub operations.

## Core Principles

### GitHub is the project-management source of truth

Do not maintain a separate task list in conversation when the work should be tracked.

Use GitHub milestones and issues to represent project state.

Before planning new work:

* inspect existing milestones
* inspect relevant open issues
* inspect existing epics
* understand what is already completed or in progress
* avoid creating duplicate issues

Prefer updating existing issues over creating duplicates.

### Human-in-the-loop is mandatory

**Never create a task automatically.**

Whenever planning results in a concrete actionable task, stop before creating the GitHub issue and ask the human for approval.

For example:

> I propose creating this task:
>
> **Implement configuration validation**
>
> * Validate required configuration at startup
> * Return actionable errors
> * Add unit tests
>
> Should I create this GitHub task?

Only create the issue after explicit approval.

This applies even when:

* the task seems obviously necessary
* the task was discovered while implementing another task
* the task was identified during code review
* the task is small
* the task was discussed earlier but not explicitly approved for creation

If several tasks are discovered, present the proposed tasks together and ask for approval before creating them.

Do not interpret general statements such as "we should do X" as permission to create an issue.

## Milestones

Milestones represent significant project phases or delivery targets.

Before creating or modifying milestones:

```bash
gh milestone list
```

Understand existing milestones before proposing new ones.

When appropriate, use milestones to group related epics and tasks.

A milestone should represent a meaningful project phase, for example:

* Foundation
* Authentication
* API Gateway
* Observability
* Production Readiness

Avoid creating milestones for individual tasks.

When creating a milestone, verify that an equivalent milestone does not already exist.

## Epics

GitHub does not have a universal native "Epic" issue type across all repositories. Use the repository's existing convention.

First inspect existing issues and labels:

```bash
gh issue list --state all
gh label list
```

If the repository uses an `epic` label, use it.

An epic represents a larger body of work that normally contains multiple tasks.

An epic should describe:

* objective
* motivation
* scope
* expected outcome
* major components
* dependencies
* acceptance criteria at a high level

Example:

```text
Epic: Authentication and Authorization

Objective:
Provide secure authentication and authorization for the gateway.

Scope:
- Authentication
- Identity propagation
- RBAC
- Authorization failures
- Integration tests

Outcome:
Requests are authenticated and authorized consistently.

Acceptance criteria:
- Unauthorized requests are rejected
- Authorized requests reach the appropriate handler
- Authorization decisions are tested
```

Do not automatically create an epic merely because one would be useful.

**Ask for approval before creating the epic.**

If the repository has an established epic workflow, follow it rather than inventing a new one.

## Tasks

Tasks are the primary units of implementation work.

A good task should be:

* actionable
* independently understandable
* small enough to implement and review
* testable
* associated with an epic when applicable
* associated with a milestone when applicable

Prefer:

> Add configuration validation for the HTTP server

over:

> Configuration

A task should normally contain:

```text
## Objective

What needs to be accomplished.

## Scope

What is included.

## Acceptance Criteria

- ...
- ...
- ...

## Testing

How the change should be verified.

## Related

Epic: #123
```

Do not make tasks unnecessarily large.

If an issue contains several independent implementation activities, consider splitting it into multiple tasks.

However, **ask the human before creating the resulting tasks.**

## Planning Workflow

When asked to plan work, follow this sequence.

### 1. Inspect project state

Start by understanding the repository:

```bash
gh repo view
gh milestone list
gh issue list --state open
gh issue list --state all
gh label list
```

Use additional `gh` commands as necessary.

Do not assume the repository's current state.

### 2. Identify existing work

Determine:

* current milestone
* active epics
* open tasks
* blocked work
* completed work
* related issues
* duplicate or overlapping work

Prefer existing issues when they already represent the required work.

### 3. Propose the plan

Present the proposed hierarchy before creating anything.

For example:

```text
Milestone: Gateway Foundation

Epic:
Authentication and Authorization

Tasks:
1. Define authorization policy model
2. Implement RBAC evaluation
3. Add authorization middleware
4. Add integration tests
```

Clearly distinguish:

* existing GitHub items
* proposed new items
* items that should be updated

### 4. Ask for approval

Before creating any new GitHub issue:

> The plan requires these new GitHub tasks:
>
> 1. ...
> 2. ...
> 3. ...
>
> Should I create them?

Wait for explicit approval.

### 5. Create approved work

After approval, create only the approved items.

Use `gh issue create`:

```bash
gh issue create \
  --title "Implement RBAC evaluation" \
  --body "..." \
  --label "task"
```

If applicable, associate the issue with the appropriate milestone.

Do not create additional issues that were not approved.

### 6. Verify

After creation:

```bash
gh issue list --state open
```

Confirm that:

* titles are correct
* labels are correct
* milestone assignment is correct
* issue relationships are represented correctly

Report the created issue numbers.

## Taking a Task to Implement

When the human asks to implement a GitHub task:

1. Retrieve the issue.
2. Read its description and acceptance criteria.
3. Inspect related epic/milestone/issues.
4. Inspect the existing code.
5. Determine the implementation approach.
6. Implement the task.
7. Run the appropriate tests and validation.
8. Update the GitHub issue with the result.
9. Close the issue only when its acceptance criteria are satisfied.

Retrieve an issue with:

```bash
gh issue view <number>
```

Do not start implementation based solely on the issue title.

## During Implementation

If implementation reveals additional work:

**Do not automatically create new issues.**

Instead, report the discovered work:

```text
While implementing #42 I discovered two additional pieces of work:

1. Add retry handling for ...
2. Add integration coverage for ...

I recommend creating these as separate tasks. Should I create them?
```

Continue implementing the current approved task when possible.

If the newly discovered work blocks the current task, explain the dependency and ask the human how to proceed.

## Updating Issues

Keep GitHub issues synchronized with actual implementation progress.

Useful commands:

```bash
gh issue comment <number> --body "..."
gh issue edit <number> ...
gh issue close <number>
```

Comments should contain meaningful project information, such as:

* implementation decisions
* discovered constraints
* test results
* blocked status
* important deviations from the original plan

Do not add noise such as comments for every minor coding step.

## Closing Tasks

Only close a task when:

* implementation is complete
* acceptance criteria are satisfied
* tests pass
* relevant validation has been performed

Before closing, summarize the result in the issue when useful.

Example:

```text
Implemented RBAC evaluation.

Completed:
- Added policy evaluation
- Added deny-by-default behavior
- Added unit tests
- Added integration coverage

Validation:
- go test ./...
- go vet ./...
```

Then close the issue:

```bash
gh issue close <number>
```

Never close an issue merely because code was written.

## GitHub CLI

Prefer `gh` over direct GitHub API calls.

Common commands:

```bash
gh repo view

gh milestone list

gh issue list
gh issue list --state all

gh issue view <number>

gh issue create

gh issue edit <number>

gh issue comment <number> --body "..."

gh issue close <number>

gh label list
```

Use `gh --help` or command-specific help when the required operation is unclear.

## Avoid Duplicate Work

Before creating an issue, search for related existing issues:

```bash
gh issue list --search "keyword"
```

Also inspect closed issues when appropriate.

If an existing issue already represents the work:

* do not create another issue
* reference the existing issue
* propose updating it if necessary

## Relationship Between Milestones, Epics, and Tasks

Use the following hierarchy:

```text
Milestone
└── Epic
    ├── Task
    ├── Task
    └── Task
```

Not every task requires an epic.

Not every project requires multiple milestones.

Use the smallest structure that accurately represents the work.

## Definition of Done

Project-management work is complete when:

* the GitHub project state has been inspected
* existing work has been considered
* proposed work has been clearly described
* the human has approved new tasks/issues
* only approved issues have been created
* implementation tasks are validated before closure
* GitHub reflects the actual state of the project

## Important Rule

**Planning proposes. The human approves. GitHub records. Implementation executes.**

Never silently turn a planning conversation into GitHub issues.
