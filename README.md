# Claude Code boilerplate — PL-driven agent team

TypeScript/Node starter where Claude Code runs as a small delivery team: a **Project Leader** (the root session) turns your requirement into a PRD and a Kanban of vertical slices, then drives **Developer**, **Reviewer**, **DevOps & Architect** and **Explorer** agents through PRs you approve.

## Quick start (developers)
```bash
npm run setup        # Node deps, Docker check/install, git init, gh auth (+ optional repo), Claude Code
npm run dev          # once the walking-skeleton task has created the app
npm run check        # typecheck + lint + tests
npm run pipeline-check   # production image → HEALTHCHECK → smoke tests (needs Docker)
docker compose up    # full local stack, once compose.yml exists
```

## Operators
Read **[GUIDE.md](GUIDE.md)** — what you write, when you approve, how to unblock, every command.

## Layout
```
CLAUDE.md                     root session = Project Leader
.claude/
  System.md  Project.md       yours (empty until you fill them)
  Requirement.md TechSpec.md  module inputs (yours)
  orgchart.md                 models / effort / parallel limits per role
  PRD/<date-slug>/            prd.md + snapshots of the inputs
  Kanban/tasks.md             the board (PL-only); archive/; log.md (local log sink, gitignored)
  history/main.md             checkpoints (local log sink, gitignored)
  agents/                     <role>.md definitions + <role>/ SOPs and git rules
  commands/                   /kickoff /next /checkpoint /resume
  hooks/                      permissions, git guard, token savers
  skills/                     grill-me, grilling, to-spec (MIT, Matt Pocock)
scripts/                      setup, log handler, board, pipeline-check
logging.config.json           log sinks (file now, Grafana Loki later)
```

Token usage: `/context`, `/cost` or `/usage` inside Claude · status line (ccusage) · `npm run tokens`, `tokens:session`, `tokens:blocks`.
