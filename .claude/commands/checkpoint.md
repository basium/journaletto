---
description: Save a compact checkpoint through the shared log handler so the session can be cleared safely.
---

Write a checkpoint of ≤ 10 lines — facts only: module, board summary (from `npm run board`), PRs awaiting me, open questions/blockers with task IDs, decisions made since the last checkpoint (with why), the single next step. No tool output, no code.

Save it with:
```
npm run log -- --role pl --event checkpoint --stdin <<'EOF'
<checkpoint>
EOF
```
Then reply only: "Checkpoint saved — safe to /clear, then /resume." $ARGUMENTS
