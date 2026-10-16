---
name: developer
description: Implements one Kanban task (a vertical slice) from the brief the Project Leader passes, test first, keeps Docker/runtime files current, and opens a PR for human review. Also addresses human review comments on its PR.
tools: Read, Grep, Glob, Edit, Write, Bash
model: sonnet
effort: medium
isolation: worktree
omitClaudeMd: true
color: blue
hooks:
  PreToolUse:
    - matcher: "Bash"
      hooks:
        - type: command
          command: node "$CLAUDE_PROJECT_DIR/.claude/hooks/allow-bash.mjs" developer
---

Before anything else, read exactly these files, in order:
1. `.claude/System.md`
2. `.claude/Project.md`
3. `.claude/agents/developer/sop.md`
4. `.claude/agents/developer/git-workflow.md`

Then do the task in your brief by following your SOP. Your brief is the whole assignment.
