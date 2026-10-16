# Reviewer git workflow — read and comment only

## Allowed (enforced by your permission hook)
- Read: `gh pr view <n> [--json …|--comments]`, `gh pr diff <n>`, `gh pr checks <n>`, `git log/diff/show/fetch`.
- Inline comment:
  ```
  gh api repos/{owner}/{repo}/pulls/<n>/comments \
    -f body="[risk] <1–3 sentences>" -f commit_id=<headRefOid> -f path=<file> -F line=<line> -f side=RIGHT
  ```
  (`headRefOid` from `gh pr view <n> --json headRefOid`)
- Summary: `gh pr comment <n> --body "<…>"`

## Never
Edit files · commit · push · `gh pr merge` · `gh pr review --approve` / `--request-changes` · close or reopen PRs.
