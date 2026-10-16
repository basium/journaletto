# Developer git workflow (trunk-based, short-lived branches)

## Allowed
`git status/diff/log/show/add/commit/fetch/rebase/switch/restore/stash/mv/rm`, push to **your own `task/*` branch only**, `gh pr create/view/diff/comment/checks`, reading/replying to PR comments via `gh api`. Everything else is blocked by your permission hook.

## Never
Push or commit to `main` · force-push without `--force-with-lease` · merge, approve or request changes on any PR · touch another task's branch · edit `.claude/Kanban/**`.

## Branch
```
git fetch -q origin
git switch -c task/<ID>-<slug> origin/main      # e.g. task/M1-T03-address-validation
```
One task = one branch = one PR. Branches live < 1 day.

## Commits
Conventional and small: `feat:`, `fix:`, `test:`, `refactor:`, `chore:` — e.g. `feat(address): validate EU postcodes (M1-T03)`.

## Sync (always fetch latest)
Run before you start, before opening the PR, and before addressing review:
```
git fetch -q origin && git rebase origin/main
```

## Conflicts
1. Resolve **in favour of the incoming change** (what is already on `main`): keep main's behaviour, then re-apply your task on top.
2. Re-read your acceptance criteria against the new `main`; adjust or recode your change so it fits the new status quo. `npm run check` must pass.
3. Note the resolution in the PR description under `## Conflict resolution` (files + what you kept).
4. **Escalate instead** — `git rebase --abort`, post a PR comment listing each conflicting file with a suggested resolution, return `CONFLICT PR #n: …` — when any of these is true:
   - checks still fail after 2 resolution attempts;
   - the same files conflict again on the next sync (loop);
   - resolving would change behaviour that another merged task depends on.

## Pull request
```
npm run check
git push -u origin HEAD
gh pr create --base main --title "<type>(<scope>): <summary> (<ID>)" --body "<What / Why / How tested, ≤ 10 lines>
Task: <ID>"
```
After review changes: push to the same branch (`git push --force-with-lease origin task/<ID>-<slug>` after a rebase).
