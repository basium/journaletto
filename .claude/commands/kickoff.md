---
description: Start a module. `/kickoff` = grill → PRD → tasks (then stop for approval). `/kickoff approved` = approve the tasks and start building.
---

Argument: $ARGUMENTS

Read `.claude/agents/project-leader/product-definition-sop.md`, then:
- If the argument is empty → run SOP §1 Intake (and §2 for writing tasks). Stop after asking me to review `.claude/Kanban/tasks.md`.
- If the argument is `approved` → run SOP §3 Approval, then §4 The loop (the walking skeleton `M*-T00` goes first when present).
