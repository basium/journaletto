# Operator guide

You are the human in this system. Claude runs a small delivery team; you supply direction, approve work and unblock it. This guide lists everything the system expects from you and every assumption it makes. (Claude is blocked from reading this file — it costs tokens and duplicates the SOPs.)

---

## 1. The team

| Role | Runs as | Does | Never does |
|---|---|---|---|
| **Project Leader (PL)** | the root Claude session you type into | Grills you, writes the PRD and tasks, dispatches agents, polls PRs, keeps the board and logs, asks you when stuck | Write feature code, merge |
| **Developer** | subagent, own worktree | One task per run: tests first, code, Docker/runtime upkeep, opens a PR, addresses your review comments, handles rebase conflicts | Touch `main`, merge, approve |
| **Reviewer** | subagent, read + comment only | ≤ 3 tagged comments + a summary that point your eyes at what matters | Approve, request changes, edit code |
| **DevOps & Architect** | subagent, own worktree | Reviews each change **after merge** with an architecture/DevOps lens; if the environment needs adjusting, opens an `env/` PR | Touch `task/` branches, merge |
| **Explorer** | subagent, read-only | Finds facts in the code for the PL | Anything else |

Models, effort and how many run in parallel: `.claude/orgchart.md` (edit freely).

---

## 2. What you own

| File | Purpose | Notes |
|---|---|---|
| `.claude/System.md` | Your general preferences (environment, style, how Claude should talk to you) | Empty until you fill it. Loaded by the PL and every agent. |
| `.claude/Project.md` | What this project is about | Empty until you fill it. Loaded by the PL and every agent. Keep it short — every agent pays for it. |
| `.claude/Requirement.md` | Module description: what and why | Overwrite for each new module. |
| `.claude/TechSpec.md` | Technical context/constraints (e.g. from the CTO) | Optional if Requirement is enough. |
| `.claude/orgchart.md` | Model / effort / parallel limits per role | Only the PL reads it. |
| `logging.config.json` | Where logs go | Add sinks later (e.g. Grafana Loki). |

Agents **never** edit these. They read Requirement/TechSpec/PRD only through the PL, who answers their questions.

---

## 3. Lifecycle of a module

```
you write Requirement.md / TechSpec.md
      │
/kickoff ── PL grills you ── PRD in .claude/PRD/<date-slug>/ ── tasks.md (Awaiting Approval)
      │
you review/edit .claude/Kanban/tasks.md
      │
/kickoff approved ── tasks → To Do ── walking skeleton M1-T00 first
      │
/next ── loop: dispatch → PR → review → you merge → DevOps check → Deployed
```

### Step by step
1. **Write the inputs.** `Requirement.md` and/or `TechSpec.md`.
2. **`/kickoff`.** The PL runs a grilling session (questions in rounds, each with a recommended answer — "yes" accepts it). Then it writes:
   - `.claude/PRD/<YYYY-MM-DD>-<summary>/prd.md` plus snapshots of your two inputs;
   - `.claude/Kanban/tasks.md`: vertical-slice tasks with IDs (`M1-T03`), dependencies, and Mode `AFK` (agent can finish alone) or `HITL` (needs you). All `Awaiting Approval`.
3. **Review the tasks.** Edit `tasks.md` directly if you like — your edits win.
4. **`/kickoff approved`.** Building starts. The first task of a new project is always `M1-T00` walking skeleton: health endpoint, Dockerfile, compose, smoke tests, build check.
5. **`/next`** runs the loop (below). Run it whenever you want progress; the PL also runs the PR part every 30 minutes while something waits on you.

### The implementation loop (per task)
1. PL picks ready tasks (all blockers Deployed), up to the orgchart's parallel limit.
2. **Developer** fetches latest `main`, writes ≤ 2 tests per acceptance criterion, implements the slice, updates Docker/runtime files if needed, opens a PR → task `Code Review`.
3. **Reviewer** leaves ≤ 3 comments tagged `[attention] [risk] [question] [scope] [test-gap]` and a summary. `[changes-suggested]` means it thinks changes are needed — **you decide**.
4. **You review on GitHub.**
   - Leave comments / request changes → the Developer picks them up automatically (next poll), fixes, pushes, replies.
   - Merge → step 5.
5. **DevOps & Architect** reviews the merged change on `main`.
   - Nothing to adjust → task **Deployed**.
   - Adjustments → it opens an `env/` PR and the PL creates a new **HITL** task "Review env PR #N". Review and merge it like any PR; the PL then runs the build check on `main` and marks it Deployed.
6. On every **Deployed**: a 2-sentence log line, the task moves to `Kanban/archive/<slug>.md`, a checkpoint is written, and the PL picks up the next ready tasks.

---

## 4. Statuses

| Status | Meaning | Who acts |
|---|---|---|
| Awaiting Approval | Task list waiting for your review | **You** → `/kickoff approved` |
| To Do | Ready once blockers are Deployed | PL |
| In Progress | A Developer is working on it | Agent |
| Code Review | PR open, waiting on your review/merge | **You** (on GitHub) |
| HITL | Needs you: credentials, a decision, a manual step, a merge conflict, or an env PR to review | **You** → `/next <ID>: <what you did>` (or merge the PR) |
| Blocked | An agent hit a blocker or a question nobody could answer from the docs | **You** → `/next <ID>: <resolution>` |
| Deployed | Merged, DevOps-checked, still runs | — |

**Unblocking:** `/next M1-T07: added SMTP key to .env` — the PL records your resolution in the task's notes and sets it back to To Do (or Deployed if it was a HITL task you completed).

---

## 5. Commands

| Command | Does |
|---|---|
| `/kickoff` | Intake: grill → PRD → tasks → stops for your approval |
| `/kickoff approved` | Approve tasks and start the loop |
| `/next` | Poll PRs, apply resolutions, dispatch ready tasks, summary |
| `/next <ID>: <text>` | Resolve a HITL/Blocked task, then continue |
| `/next --poll` | PR check only (what the 30-minute check runs) |
| `/checkpoint` | Save a ≤ 10-line checkpoint (safe to `/clear` after) |
| `/resume` | After `/clear` or a new session: last checkpoint, board, restart the PR check |
| `/grill-me` | Stress-test any plan ad hoc |

Also: `npm run board` (compact board), `npm run log -- …` (PL's log handler), `npm run pipeline-check`.

---

## 6. Git & PR rules (enforced by hooks)

- Trunk-based: `main` always releasable; short-lived branches `task/<ID>-<slug>` (Developer) and `env/<ID>-<slug>` (DevOps); squash-merge.
- **Only you merge.** Nobody (PL included) can `gh pr merge`, approve, or push code to `main`.
- The PL may commit **only** `.claude/Kanban/**` and `.claude/PRD/**` straight to `main`. If your GitHub branch protection blocks direct pushes, allow it for your account or the board will stay local.
- Agents use your `gh` login, so GitHub won't let them approve or request changes on PRs (they are the author). Reviewer comments are the signal. Optional later: a separate bot account/GitHub App for agents.
- **Conflicts:** the Developer rebases on latest `main` before starting, before the PR, and before addressing review. It resolves conflicts in favour of what's already on `main` and re-fits its change. It hands the conflict to you (HITL, with suggestions on the PR) if checks still fail after 2 attempts, the same files conflict again, or resolving would change behaviour another merged task relies on.
- Per-role permissions: each agent has its own Bash allowlist (`.claude/hooks/bash-allowlist.json`). Explorer has no shell; Reviewer can only read and comment.

---

## 7. Runnability & the build check

- **Developers own Docker**: any task that adds a dependency, env var, port or service updates `Dockerfile`, `compose.yml`, `compose.ci.yml`, `.env.example`/`.env.ci` and the smoke test in the same PR.
- **`npm run pipeline-check`** mirrors a real pipeline: build the production image (`runtime` target) → start deps from `compose.ci.yml` → run with `.env.ci` → wait for the Docker `HEALTHCHECK` → 1–3 smoke tests → teardown.
- It runs **only for environment changes**: CI on `env/*` PRs or PRs touching Dockerfile/compose/`.env.ci`/CI/`package.json`/lockfile; locally by the agent who touched those files; by the PL after an env PR merges.
- No Docker? It falls back to a local build + start + `/health` check and warns that the Docker path wasn't verified. Install Docker (setup offers it).
- The production image is your deployment artifact: `docker build --target runtime .` works in any pipeline. Staging/production deployment is out of scope until you add it to a tech spec. Trivy scan and registry push are in `pipeline-check.yml`, switched off.

---

## 8. Logs, history and debugging

- All activity goes through **one handler**: `scripts/log.mjs`, sinks in `logging.config.json`.
  - `.claude/Kanban/log.md` — every task move, agent close-line (timestamp · role · task · PR · one sentence), blocker, resolution, deployment. Gitignored, append-only.
  - `.claude/history/main.md` — checkpoints. Gitignored.
  - Grafana Loki sink present, `"enabled": false`. New destination = new sink entry (+ small adapter for a new type).
- These are **for you** (and for pasting into an AI when debugging), not for automation. Claude is blocked from reading them; the PL reads only the latest checkpoint via `node scripts/log.mjs --last checkpoint`.
- Archived (Deployed) tasks: `.claude/Kanban/archive/<date-slug>.md`. PRDs and input snapshots: `.claude/PRD/<date-slug>/`.

---

## 9. The 30-minute PR check

- Starts automatically when a task enters Code Review or HITL-with-PR; stops when nothing waits on you.
- It lives **inside the running Claude session** — close Claude and it stops. `/resume` restarts it if anything is still waiting.

---

## 10. Token efficiency (built in)

- Root `CLAUDE.md` is tiny; agents skip it (`omitClaudeMd`) and read only System, Project, their SOP and git rules, plus the one task block the PL hands them.
- PRD/Requirement/TechSpec are never read by agents; the PL answers their questions and resumes them (no lost context).
- `npm run board` instead of re-reading `tasks.md`; Deployed tasks are archived out of it.
- Hooks: `quiet-bash` (blocks output floods like `cat` of whole files, raw test runners, unbounded `git log`), `context-nudge` (tells you to `/checkpoint` → `/clear` → `/resume` past ~80k tokens; tune `CLAUDE_NUDGE_TOKENS`), `inject-history` (latest checkpoint after compaction).
- Cheap models where it's safe (Explorer on Haiku, low effort for Reviewer); `effortLevel: medium` by default.
- Watch usage: `/context`, `/cost` or `/usage`, the ccusage status line, `npm run tokens*`, VS Code task "Token usage: live 5h block".

---

## 11. Assumptions (all decisions from the design session)

1. The root session is the PL; subagent nesting is disabled (`CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1`) so control stays with the PL.
2. Agent definitions live in `.claude/agents/<role>.md`; SOP folders beside them are documentation (no frontmatter, ignored as agents).
3. Waiting for you is a status, not a running agent; the PL polls PRs every 30 min while needed.
4. Only the PL writes `tasks.md` and logs; agents return a one-sentence close line that the PL logs.
5. `tasks.md` and PRDs are committed; logs and history are local (gitignored) through the shared handler.
6. Requirement and TechSpec are human-controlled module descriptions; agents work from tasks.
7. HITL is its own status.
8. "Deployed" = merged + DevOps-reviewed + still runs; no staging/production yet; the Dockerfile is deployable by your own pipeline.
9. Trunk-based, one PR per task (plus an optional env PR), squash-merge, only you merge.
10. Models/effort/parallelism live in `orgchart.md`.
11. Package quick-start configs, Jev plugin and all token-saving tooling carried over.
12. Developers own Docker; DevOps reviews post-merge only and raises env PRs as HITL tasks.
13. Developers always fetch latest; conflict policy as in §6.
14. Each role has its own permissions; nobody approves or merges.
15. The build check mirrors a real pipeline and runs only for environment changes; Docker recommended, local fallback.
16. Tests: ≤ 2 per acceptance criterion; Reviewer ≤ 3 comments.
17. Agents ask questions via the PL; the PL answers from the docs or escalates to you as Blocked.
