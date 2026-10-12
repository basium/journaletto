# Journal Recall

## Authoritative documents

- Requirements: `docs/requirements.md`
- Architecture decisions: `docs/decisions/`
- Requirement/test mapping: `docs/traceability.md`

Read the relevant documents before planning or implementation.

## Current priority

Milestone 1: Standalone Journal Entry Service.

Do not implement later milestones without explicit instruction.

## Engineering rules

- Implement one vertical slice at a time.
- Raw Markdown files are the source of truth.
- Saving raw files must not depend on model availability.
- Map implementation and tests to requirement IDs.
- Use real SQLite, FTS5 and temporary filesystem directories in integration tests.
- Mock only model execution and external providers.
- Do not silently alter product requirements.
- Commit freely to the active PR branch; every PR requires human administrator approval before merge (see Development and GitHub Workflow).

## Completion requirements

Before declaring work complete:

1. Run formatting, static analysis and relevant tests.
2. Update `docs/traceability.md`.
3. Report requirements completed, tests run, assumptions and remaining risks.
## Development and GitHub Workflow

### 1. Mandatory Rules

- Never commit or push directly to `main`.
- All code changes must go through a pull request targeting `main`.
- Maintain only one active Claude-managed pull request at a time.
- All subsequent changes must be added to the existing active PR until a human administrator merges it.
- Only a human repository administrator may approve and merge a PR.
- Never approve, merge, or close your own PR.
- Never enable auto-merge or bypass branch protection.
- Never force-push or rewrite shared Git history.
- Never discard existing user changes.
- If any rule cannot be followed safely, stop and ask the administrator.

### 2. Before Starting Any Task

Always inspect the repository state before modifying files.

1. Run `git status`.
2. Run `git branch --show-current`.
3. Run `git fetch origin`.
4. Check whether a Claude-managed PR is already open.
5. Inspect its source branch, description, review comments, and CI status.

Use the GitHub CLI to inspect open PRs:

```bash
gh pr list --state open --base main
```

Never assume that the current branch is correct or that a previous PR has been merged.

### 3. Existing Active PR

If a Claude-managed PR is open:

- Continue working on its existing source branch.
- Do not create another branch or PR.
- Implement new requested changes on the same branch.
- Run relevant tests and inspect the diff.
- Commit changes and push them to the same branch.
- Verify that the existing PR contains the latest commits.
- Report the PR URL and a summary of the changes.

New commits automatically update the existing PR when pushed to its source branch.

If the PR is awaiting review or has been approved but not merged, continue using that PR.

If review feedback requests changes, address the feedback on the same branch.

If a new task is unrelated to the existing PR, ask the administrator whether to defer it. Do not mix unrelated work into the PR without permission.

### 4. Creating a New PR

Create a new PR only after confirming that the previous Claude-managed PR has been merged by a human administrator.

1. Fetch the latest repository state.
2. Confirm the previous PR's state is MERGED.
3. Update the local `main` branch from `origin/main`, preserving uncommitted changes.
4. Create a new feature branch from the updated `main`.
5. Implement the requested changes.
6. Run tests, linting, and relevant build checks.
7. Review the diff for unintended changes.
8. Commit and push the feature branch.
9. Create a PR targeting `main`.
10. Report the PR URL and stop.

Example:

```bash
git fetch origin
git switch main
git pull --ff-only origin main
git switch -c feature/short-description
```

Only run these commands when the working tree is safe and the previous PR has been confirmed as merged.

### 5. Approval and Merge Restrictions

Claude must never:

- Approve its own PR.
- Merge any PR.
- Use `gh pr merge` or another mechanism to merge changes.
- Enable auto-merge.
- Modify repository rules or permissions to bypass approval.
- Push directly to protected branches.

After creating or updating a PR, stop and wait for the human administrator.

The administrator is responsible for reviewing the diff, approving the PR, and merging it.

Do not create a new PR simply because the current PR has received approval. Wait until it is actually merged.

### 6. Code Quality

Before pushing changes:

- Run the relevant automated tests.
- Run linting and build checks where available.
- Inspect the complete diff.
- Check for exposed secrets, credentials, and sensitive configuration.
- Avoid unrelated refactoring.
- Update documentation when behaviour or setup instructions change.
- Never claim that tests passed unless they were actually executed successfully.

Every PR description must include:

- Summary of the changes.
- Reason for the changes.
- Tests performed and their results.
- Risks, limitations, and deployment considerations.

### 7. Protect Existing Work

Before switching branches or performing Git operations:

- Check for uncommitted changes.
- Preserve user modifications.
- Never run destructive commands such as `git reset --hard` or `git clean -fd` without explicit permission.
- Do not overwrite commits or changes made by another contributor.
- If the correct branch or PR is unclear, stop and ask.

### 8. Final Principle

One active Claude-managed PR at a time.

All new work goes into the existing PR until a human administrator merges it. Only then may a new PR be created.

Claude implements, tests, commits, pushes, and opens PRs. A human administrator alone approves and merges the work.
