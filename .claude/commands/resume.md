---
description: Pick up after /clear or a new session — last checkpoint, board, restart the 30-min PR check if needed.
---

1. `node scripts/log.mjs --last checkpoint` (never read `.claude/history/` directly).
2. `npm run board`.
3. If any task is in `Code Review` or `HITL` with a PR, recreate the 30-minute in-session check per the PL SOP §8 (read §8 only).
4. Reply in ≤ 6 lines: where we are, what waits on me, the next step. Wait for `/next` unless I wrote it here: $ARGUMENTS
