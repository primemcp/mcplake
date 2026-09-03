---
name: gitflow
description: >
  Follow this GitFlow process for all development work.
---

# GitFlow Development

Follow this GitFlow process for all development work.

---

## Branch Model

The repository uses two permanent branches:

* `main` — release/production branch
* `develop` — latest development branch

Never develop directly on `main` or `develop`.

All implementation work must happen on a feature branch created from `develop`.

## Ticket-Driven Development

Every feature branch must correspond to an approved GitHub task from the project-management workflow.

Use the GitHub ticket number in the branch name.

Branch naming convention:

```text
feature/<ticket_number>_<short_title>
```

Examples:

```text
feature/123_add_rbac
feature/145_config_validation
feature/201_health_endpoint
```

The ticket number must come from the GitHub project-management task.

Do not invent ticket numbers.

Do not create a feature branch for untracked work. If the required task does not exist in GitHub, follow the project-management skill and obtain human approval before creating it.

## Starting a Task

Before starting implementation:

1. Identify the approved GitHub task.
2. Ensure the local repository is up to date.
3. Switch to `develop`.
4. Update `develop`.
5. Create the feature branch from `develop`.

Example:

```bash
git switch develop
git pull --ff-only origin develop
git switch -c feature/123_add_rbac
```

Confirm that the branch was created from the current `develop`.

## Work Only on Feature Branches

After creating the feature branch:

```bash
git branch --show-current
```

Verify that the current branch is the expected:

```text
feature/<ticket_number>_<short_title>
```

Do not commit implementation changes directly to:

* `main`
* `develop`

If currently on either protected branch, create the appropriate feature branch before making implementation commits.

## Rebase Before Committing

Before creating a commit, synchronize the feature branch with the latest `develop` as much as possible.

The standard workflow is:

```bash
git add .
git stash
git pull
git stash pop
```

The purpose is to minimize conflicts by incorporating the latest repository state before committing.

After restoring the changes:

```bash
git status
```

Inspect the working tree and resolve any conflicts before proceeding.

If `git stash pop` produces conflicts:

1. Stop.
2. Resolve the conflicts carefully.
3. Verify the resulting code.
4. Run the relevant tests.
5. Continue only after the working tree is correct.

Do not blindly accept either side of a conflict.

### Important

The preferred workflow is to synchronize before committing.

Do not create a commit first and then discover that the branch is significantly behind `develop` when avoidable.

## Committing

Create focused commits.

Before committing:

```bash
git status
git diff
git diff --cached
```

Run the appropriate tests and validation before committing.

Commit messages should clearly describe the change.

Example:

```bash
git commit -m "Add RBAC policy evaluation"
```

Where appropriate, include the GitHub ticket number in the commit message.

Example:

```bash
git commit -m "#123 Add RBAC policy evaluation"
```

Do not create meaningless commits such as:

```text
fix
changes
update
stuff
```

## If Push Fails or the Branch Is Behind

If code has already been committed and pushing reveals that the remote/develop history has moved or the branch needs synchronization, use rebase.

First fetch the latest state:

```bash
git fetch origin
```

Then rebase the feature branch onto the latest `develop`:

```bash
git rebase origin/develop
```

Resolve conflicts if necessary.

After resolving each conflict:

```bash
git add <resolved-files>
git rebase --continue
```

Repeat until the rebase completes.

Then verify:

```bash
git status
git log --oneline --graph --decorate -n 20
```

Because rebase rewrites commit history, pushing the rebased branch may require:

```bash
git push --force-with-lease
```

Prefer `--force-with-lease`.

**Never use ****`git push --force`**** unless explicitly instructed by the human.**

## Pull Requests

All feature branches merge into `develop` through the standard GitHub Pull Request process.

Do not merge feature branches directly into `develop` from the local command line.

Create the PR using `gh` when appropriate:

```bash
gh pr create \
  --base develop \
  --head feature/123_add_rbac
```

The PR should:

* reference the GitHub task
* explain what was implemented
* describe relevant design/implementation decisions
* describe testing performed
* identify any known limitations

A PR should normally contain only the work associated with its ticket.

## Human Review Is Mandatory

**Never merge a Pull Request without human review and approval.**

The workflow is:

```text
GitHub Task
    ↓
Feature Branch
    ↓
Implementation
    ↓
Tests
    ↓
Commit
    ↓
PushManage project planning and execution through GitHub using the `gh` CLI.
Use milestones, epics, and tasks to turn plans into tracked, actionable work.
Always keep the human in the loop before creating tasks or issues.
    ↓
Pull Request → develop
    ↓
Human Review
    ↓
Approval
    ↓
Merge
```

Claude may:

* create the feature branch
* implement the task
* commit changes
* push the feature branch
* create the PR
* inspect CI results
* respond to review feedback

Claude must not bypass the human review process.

Do not approve your own PR.

Do not merge a PR merely because CI passes.

Do not use administrative/bypass mechanisms to circumvent branch protection or required reviews.Manage project planning and execution through GitHub using the `gh` CLI.
Use milestones, epics, and tasks to turn plans into tracked, actionable work.
Always keep the human in the loop before creating tasks or issues.

## Pull Request Review Feedback

When human review identifies changes:

1. Update the feature branch.
2. Make the requested changes.
3. Run tests.
4. Commit the changes.
5. Push the branch.
6. Allow the PR to update automatically.

Do not create a new PR for ordinary review feedback.

If the branch has become significantly behind `develop`, rebase it:

```bash
git fetch origin
git rebase origin/develop
git push --force-with-lease
```

## Merging to Develop

The feature branch is merged into `develop` only through the approved Pull Request process.

After the PR is merged:

```bash
git switch develop
git pull --ff-only origin develop
```

Do not continue development on the old feature branch after its work has been merged.

Delete the local feature branch when appropriate:

```bash
git branch -d feature/123_add_rbac
```

The remote branch should also be deleted through the normal GitHub PR workflow when appropriate.

## Main Is the Release Branch

`main` represents released/production code.

Normal feature development must never target `main`.

The normal flow is:

```text
feature/<ticket>
        ↓
      develop
        ↓
     release
        ↓
      main
```

`develop` contains the latest integrated development work.

`main` contains released versions.

Release procedures should be handled separately from normal feature development.

## Protect the Branches

Treat both permanent branches as protected:

```text
main
develop
```

Never:

```bash
git commit directly on main
git commit directly on develop
git push directly to main
git push directly to develop
```

unless the repository's explicitly defined release or administrative process requires it.

## Conflict Resolution

When conflicts occur:

1. Understand why the conflict exists.
2. Inspect both sides.
3. Preserve the intended behavior from both changes where appropriate.
4. Resolve manually.
5. Run formatting and tests.
6. Inspect the resulting diff.
7. Continue the rebase or commit only after verification.

Never resolve conflicts by blindly choosing:

```bash
git checkout --ours
```

or:

```bash
git checkout --theirs
```

without understanding the code.

## Standard Task Workflow

For a normal GitHub task, follow this sequence:

```text
1. Read GitHub ticket
        ↓
2. Switch to develop
        ↓
3. Pull latest develop
        ↓
4. Create feature/<ticket>_<short_title>
        ↓
5. Implement
        ↓
6. Test
        ↓
7. Synchronize before commit:
      git add .
      git stash
      git pull
      git stash pop
        ↓
8. Resolve conflicts if necessary
        ↓
9. Test again
        ↓
10. Commit
        ↓
11. Push feature branch
        ↓
12. Create PR → develop
        ↓
13. Human review
        ↓
14. Address review feedback
        ↓
15. Human-approved merge → develop
        ↓
16. Update local develop
```

## Non-Negotiable Rules

1. `main` is the release branch.
2. `develop` is the latest development branch.
3. Feature branches are always created from `develop`.
4. Feature branches use `feature/<ticket_number>_<short_title>`.
5. Every feature branch must correspond to a GitHub task.
6. GitHub provides the ticket number.
7. Do not invent or reuse ticket numbers.
8. Do not commit directly to `main` or `develop`.
9. Synchronize before committing using the project's standard stash/pull workflow.
10. If synchronization is needed after commits, use `git rebase`.
11. Resolve conflicts deliberately and test afterward.
12. Use `git push --force-with-lease` after rebasing when necessary.
13. Never use plain `git push --force` without explicit human instruction.
14. Merge feature branches into `develop` through a Pull Request.
15. Human review is mandatory before merging.
16. Never bypass required GitHub review or branch protection.
17. Keep GitHub ticket, branch, commit, and PR associated with the same piece of work.
