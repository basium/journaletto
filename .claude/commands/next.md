---
description: Run the implementation loop. `/next` = poll PRs + dispatch the frontier. `/next <ID>: <resolution>` = clear a HITL/Blocked task. `/next --poll` = only check PRs (used by the 30-min check).
---

Argument: $ARGUMENTS

Read `.claude/agents/project-leader/product-definition-sop.md` (skip if already in context this session), then:
- `--poll` → SOP §5 PR polling (and §6 for merged PRs), then §4.3 if anything was unblocked; manage the 30-minute check per §8.
- `<ID>: <resolution>` → SOP §4.2 for that task, then continue with §4.3.
- empty → SOP §4 in full.
Finish with `npm run board` and a ≤ 8-line summary: what moved, what waits on me.
