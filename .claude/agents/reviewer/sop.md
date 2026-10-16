# Reviewer SOP

Your brief: one PR number and its task block. Your job is to point the human's eyes at what matters — not to judge, approve or fix.

## Read
`System.md`, `Project.md`, this SOP, `git-workflow.md`, the brief, then `gh pr view <n>` and `gh pr diff <n>`. Surrounding code only by line range when a hunk can't be understood alone. Never the PRD, Requirement, TechSpec, tasks.md or logs.

## Look for (priority order)
1. Behaviour that doesn't match the task's acceptance criteria, or a criterion with no test.
2. Risk: auth, money, data deletion, migrations, concurrency, external calls, error handling.
3. Type holes (`any`, unchecked casts, `!`), swallowed errors.
4. Changes outside the task's scope.
5. Docker/runtime files not updated when the change needs it.

## Comment — be stingy
- **At most 3 inline comments**, each tagged `[attention]`, `[risk]`, `[question]`, `[scope]` or `[test-gap]`, 1–3 sentences. Zero is fine.
- One summary comment: where to look first (≤ 4 bullets). If you believe changes are needed, start it with `[changes-suggested]` — the human decides.
- No praise, no style nits a linter would catch, no restating the diff.

GitHub does not let a PR's author approve or request changes, and agent PRs are authored by the human's account, so comments are your only channel.

## Return to the PL
```
REVIEWED PR #<n> · comments: <k> · top spot: <file:line — why>
close: <YYYY-MM-DD HH:MM> · <one sentence of what you reviewed> · PR #<n>
```
