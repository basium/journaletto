# Boilerplate recreation spec

> Feed this file to an LLM (with file-writing tools) to recreate the boilerplate. It captures every preference and decision from the design sessions with Alessio (Oct 2026). Where it says MUST, the behaviour was explicitly chosen; where it says "e.g.", the exact wording is free.

## 1. Goal and principles
A VS Code + Claude Code starter repository (TypeScript/Node) where Claude works as a **small delivery team** driven by a **Project Leader (PL)**, from a human-written requirement to merged, runnable, Dockerised software. Priorities, in order:
1. **Token efficiency** everywhere: small always-loaded context, agents receive the minimum, nothing is read "just in case".
2. **Human control**: only the human merges/approves; human owns requirements, system and project descriptions.
3. **Vertical slices** with feedback after each.
4. **Quick start** for developers: install, run in dev mode or with Docker, immediately.
5. **Debuggability**: short logs of every move, kept for humans (to paste into an AI later), not for automation.

## 2. Repository layout (MUST)
```
CLAUDE.md                         root-only router; root session = PL
README.md                         short developer quick start + layout
GUIDE.md                          operator guide for the human (Claude denied reading it)
BOILERPLATE_SPEC.md               this file (Claude denied reading it)
package.json  tsconfig.json  tsconfig.build.json  eslint.config.js  .prettierrc  .prettierignore
.editorconfig  .nvmrc (22)  .gitignore  .env.example  logging.config.json
src/index.ts                      placeholder (`export {};`) so typecheck passes; walking skeleton replaces it
.vscode/  settings.json  extensions.json  tasks.json
.github/  workflows/ci.yml  workflows/pipeline-check.yml  pull_request_template.md
scripts/  setup.sh  log.mjs  board.mjs  pipeline-check.sh
.claude/
  System.md                       EMPTY — human writes it (general preferences)
  Project.md                      EMPTY — human writes it (what the project is)
  Requirement.md                  human-owned module description (template comment only)
  TechSpec.md                     human-owned technical context/constraints, e.g. from the CTO (template comment only)
  orgchart.md                     model/effort/parallel limit per role — read only by the PL
  settings.json                   permissions, env, hooks, status line, Jev plugin
  PRD/<YYYY-MM-DD>-<slug>/        prd.md + requirement.snapshot.md + techspec.snapshot.md
  Kanban/tasks.md                 the board (PL-only writer, committed)
  Kanban/archive/<date-slug>.md   Deployed tasks moved out of tasks.md
  Kanban/log.md                   local log sink (gitignored)
  history/main.md                 checkpoints, local log sink (gitignored)
  agents/
    developer.md        developer/sop.md   developer/git-workflow.md
    reviewer.md         reviewer/sop.md    reviewer/git-workflow.md
    devops-architect.md devops-architect/sop.md  devops-architect/environment.md
    explorer.md         explorer/sop.md
    project-leader/product-definition-sop.md     (no definition file — the PL is the root session)
  commands/  kickoff.md  next.md  checkpoint.md  resume.md      (only these four)
  hooks/  guard-git.mjs  allow-bash.mjs  bash-allowlist.json  quiet-bash.mjs  context-nudge.mjs  inject-history.mjs
  skills/  grill-me/  grilling/  to-spec/     (vendored from mattpocock/skills, MIT, LICENSE copied)
```
Folder names are lowercase (macOS is case-insensitive; `.claude/agents` is the official directory). SOP files have **no frontmatter**, so Claude Code treats them as documentation, not agents (it scans `agents/` recursively).

## 3. Roles
| Role | How it runs | Default model / effort / parallel |
|---|---|---|
| Project Leader | the **root session** (talks to human, schedules checks, spawns agents) | session model |
| Developer | subagent `developer`, `isolation: worktree` | sonnet / medium / 3 |
| Reviewer | subagent `reviewer` | sonnet / low / 3 |
| DevOps & Architect | subagent `devops-architect`, `isolation: worktree` | sonnet / medium / 1 |
| Explorer | subagent `explorer` | haiku / low / 2 |
These live in `orgchart.md`; the PL passes model/effort when spawning (overrides agent-file defaults). Set `CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1` so agents can't spawn agents.

Agent definition files (frontmatter): `name`, `description`, `tools`, `model`, `effort`, `isolation` (dev/devops), `omitClaudeMd: true`, `color`, and a frontmatter `hooks.PreToolUse` (matcher `Bash`) running `node "$CLAUDE_PROJECT_DIR/.claude/hooks/allow-bash.mjs" <role>`. Body: "read exactly System.md, Project.md, your SOP, your git file (DevOps: environment.md), then follow the SOP." Explorer tools: Read, Grep, Glob only (no Bash). Reviewer: Read, Grep, Glob, Bash (allowlisted). Dev/DevOps: Read, Grep, Glob, Edit, Write, Bash.

## 4. What each role reads (context policy)
- **Root/PL**: `CLAUDE.md` imports `@.claude/System.md` and `@.claude/Project.md`; reads its SOP when running commands; reads Requirement/TechSpec/PRD to write the PRD/tasks and answer agents' questions.
- **Agents**: System.md, Project.md, own SOP, own git rules, and the **single task block** the PL passes. Nothing else by default. Never PRD/Requirement/TechSpec/tasks.md/logs/history. Code by line range only. Developer may read `devops-architect/environment.md` only when touching runtime files.
- **Questions**: agent stops and returns `QUESTION: …`; PL answers from PRD/TechSpec and **resumes the same agent with `SendMessage`** (context intact). If unanswerable → task Blocked → human.

## 5. Flow
### Intake — `/kickoff`
1. PL reads Requirement.md and/or TechSpec.md (one required, both if present).
2. Runs the vendored `grilling` skill (rounds of numbered questions, each with a recommended answer; "yes" accepts) until no open decisions remain.
3. Slug `<YYYY-MM-DD>-<2–4 word summary>`; creates `.claude/PRD/<slug>/`, copies inputs as `*.snapshot.md`.
4. `/to-spec` (adapted to write `.claude/PRD/<slug>/prd.md`).
5. Writes tasks to `tasks.md` (format §6), all `Awaiting Approval`; logs; checkpoints; commits the board; **stops** for human review.
### Approval — `/kickoff approved`
All → `To Do`; first task of a project is `M1-T00` **walking skeleton** (by a Developer): trivial endpoint, `GET /health` (200 + version/commit; non-HTTP: closest equivalent), multi-stage Dockerfile with `runtime` target + `HEALTHCHECK`, `compose.yml`, `compose.ci.yml` (deps), `.env.ci`, `npm run start`, `npm run test:smoke`, passing `scripts/pipeline-check.sh`. Everything else is blocked by it.
### Loop — `/next`
1. Poll PRs awaiting the human (`gh pr view --json state,reviewDecision,mergedAt,reviews,comments`).
2. Apply `/next <ID>: <resolution>` (note in task, back to To Do, or Deployed if a finished HITL task).
3. Frontier = To Do tasks whose blockers are all Deployed. HITL-mode → status HITL + tell the human. AFK → In Progress, spawn Developer (background, worktree) with the task block, within orgchart limits.
4. Developer returns `DONE PR #n` → Code Review, spawn Reviewer on the PR, ensure the 30-min check exists. `QUESTION` → §4. `BLOCKED` → Blocked + notify. `CONFLICT PR #n` → HITL + notify.
5. Human comments/requests changes → PL resumes/re-spawns Developer "Address review on PR #n" (task stays Code Review). Developer syncs with main, fixes, pushes, replies per comment (acts only on the human's comments).
6. **PR merged** → PL updates main, spawns DevOps with task ID, PR number, merge SHA. DevOps reviews the merged change on main (architecture, runnability, deployability, CI):
   - `NO_CHANGES` → task Deployed.
   - `ENV_PR #m` (DevOps implemented adjustments on `env/<ID>-<slug>`) → task Deployed **and** PL creates a new task "Review env PR #m (from <ID>)" with Mode/Status HITL. When it merges, PL runs `scripts/pipeline-check.sh` on main, then Deployed (fail → Blocked).
7. On every Deployed: log ≤ 2 sentences, move block to `Kanban/archive/<slug>.md`, commit board, `/checkpoint`, collect agents' close lines into the log, continue with the next frontier.
8. Summary to human ≤ 8 lines.
### 30-minute PR check
PL creates an **in-session** recurring scheduled check (e.g. CronCreate) running `/next --poll` only while some task is Code Review or HITL-with-PR; deletes it when nothing waits. It dies with the session; `/resume` recreates it.
### Blockers
Any agent blocker → task Blocked, reason in Notes, log, notify the human in the root session. Human resolves via `/next <ID>: <resolution>` → To Do.

## 6. Kanban
- `tasks.md` holds **active** tasks only; written **only by the PL**; committed to git (PL may commit `.claude/Kanban/**` and `.claude/PRD/**` directly to main — nothing else).
- Statuses: `Awaiting Approval` → `To Do` → `In Progress` → `Code Review` → `Deployed`, plus `HITL` and `Blocked` (back to To Do when resolved). No "Env Change" status.
- Mode: `AFK` | `HITL`. IDs `M<module>-T<NN>` in dependency order.
- Block format:
  ```
  ## M1-T03 · <title>
  Status: To Do · Mode: AFK · Assignee: — · Blocked by: M1-T01 · PR: —
  What: <end-to-end behaviour, user's perspective>
  Acceptance:
  - [ ] <criterion>
  Notes: <PRD/TechSpec knowledge copied in, ≤ 5 lines>
  ```
- Tasks are **vertical slices** (thin path through every layer, demoable), self-sufficient (agent needs nothing else), ≤ ~150 changed lines; wide refactors use expand → migrate → contract.
- `scripts/board.mjs` (`npm run board`): counts per status, frontier, items waiting on the human — so the PL needn't re-read the file.

## 7. Git workflow (trunk-based)
- `main` always releasable; short-lived branches `task/<ID>-<slug>` (Developer) and `env/<ID>-<slug>` (DevOps); one task = one PR; squash-merge; conventional commits with the task ID.
- Developers **always fetch latest**: `git fetch -q origin && git rebase origin/main` before starting, before opening the PR, before addressing review.
- **Conflicts**: resolve in favour of the incoming change (main), re-check acceptance criteria against new main, recode to fit; document under "Conflict resolution" in the PR. Escalate (abort rebase, PR comment with per-file suggestions, return `CONFLICT`) when checks fail after 2 attempts, the same files conflict again, or resolution would change behaviour another merged task relies on.
- **Only the human merges/approves.** Agents use the human's `gh` login, so GitHub won't let them approve/request changes (they're the author) — Reviewer is comment-only, signalling with `[changes-suggested]`. Optional later: separate bot account/GitHub App.

## 8. Role SOP essentials
- **Developer**: tests first, **max 2 tests per acceptance criterion**, highest seam, behaviour only, no snapshot/trivial/framework tests; strict TS, no `any`; no out-of-scope changes; **owns Docker/runtime files** (deps, env vars, ports, services → Dockerfile, compose, `.env.example`/`.env.ci`, smoke test in the same PR); `npm run check` after each criterion, max 3 fix attempts → BLOCKED; runs `pipeline-check.sh` only if it touched build/env files; opens PR via `gh pr create` without human intervention. Return format: first line `DONE PR #n | QUESTION: … | BLOCKED: … | CONFLICT PR #n: …`, then check results, then `close: <YYYY-MM-DD HH:MM> · <one sentence> · PR #n`.
- **Reviewer**: priority = criteria mismatch/missing tests, risk (auth, money, deletion, migrations, concurrency, external calls, errors), type holes, scope creep, runtime files not updated. **≤ 3 inline comments**, tagged `[attention] [risk] [question] [scope] [test-gap]`, plus one summary (≤ 4 bullets). No praise or lint nits. Inline via `gh api repos/{owner}/{repo}/pulls/<n>/comments -f body -f commit_id -f path -F line -f side=RIGHT`. Return `REVIEWED PR #n · comments: k · top spot` + close line.
- **DevOps & Architect**: post-merge review only (no pre-merge comments); lens = architecture boundaries/coupling/back-compat, runnability, deployability (small non-root reproducible env-configured runtime image usable in the human's own pipeline), CI coverage; runs pipeline-check only if infra files changed; fixes environment via env PR; returns `NO_CHANGES | ENV_PR #m | QUESTION | BLOCKED` + close line. Git rules are in its SOP (env/* only). Maintains `environment.md` (conventions below).
- **Explorer**: read-only, ≤ 15 lines, answer + `path:line`.
- **PL**: everything in §5–§6; never writes feature code; brief = one task block, never conversation history.

## 9. Environment & build check
- Local dev: `npm install` → `npm run dev`; `docker compose up` for the stack; config via env vars only; `.env.example` complete.
- Dockerfile: `deps` → `build` → `runtime` (prod deps, non-root, `NODE_ENV=production`, `HEALTHCHECK` hitting `/health`).
- `scripts/pipeline-check.sh` (same in CI and locally; mirrors a pipeline): build `--target runtime` → `docker compose -f compose.ci.yml up -d --wait` if present → `docker run --env-file .env.ci` → wait for Docker health = healthy (90 s) → `BASE_URL=… npm run test:smoke` → logs on failure, always teardown. Skips if no Dockerfile yet. **No Docker → fallback** to `npm run build && npm run start` + `/health` with a warning. Bash 3.2-safe (macOS).
- **Runs only for environment changes**: CI `pipeline-check.yml` uses `dorny/paths-filter` — runs on `env/*` PRs or PRs touching `Dockerfile`, `compose*.yml`, `.env.ci`, `scripts/pipeline-check.sh`, `.github/workflows/**`, `package.json`, lockfile (plus manual dispatch). Locally: by the agent who touched those files; DevOps under the same rule; PL after env PRs merge. Trivy scan + registry push steps present with `if: false`.
- `ci.yml`: npm ci → `npm run check` on every PR and push to main.
- No staging/production deployment yet; the runtime image is the deployable artifact.

## 10. Logging (shared handler)
- `scripts/log.mjs` is the **single entry point**: `npm run log -- --role <r> --task <ID> --pr <n> --event <e> "<msg>"` (`--stdin` for multi-line; `--last <event>` prints the newest entry). Events: kickoff, task-move, close, question, blocker, resolved, deployed, checkpoint, note.
- `logging.config.json` lists sinks: file `.claude/Kanban/log.md` (format `line`, all events except checkpoint), file `.claude/history/main.md` (format `block`, checkpoints), `loki` sink (Grafana Loki push API) `enabled: false`. New destination = new sink (+ adapter for a new type). Local timestamps `YYYY-MM-DD HH:MM`.
- Line format: `2026-11-26 14:05 · developer · M1-T03 · PR #42 · close · Changed address validation · PR #42`.
- Only the PL calls it. Log files gitignored. Claude is denied reading them; the PL only uses `--last checkpoint`.

## 11. Permissions & hooks
- **settings.json**: `model: sonnet`, `effortLevel: medium`; allow-list for the union of agent commands (npm/npx/node scripts, pipeline-check, docker, git read/commit/fetch/rebase/switch, pushes to `task/*`, `env/*`, `-u origin HEAD`, `origin main`, gh pr view/diff/checks/list/create/comment, `gh api repos/*/pulls/*`); ask for other pushes and `npm install`; deny `gh pr merge` and reads of GUIDE.md, BOILERPLATE_SPEC.md, Kanban/log.md, Kanban/archive, history, node_modules, dist, coverage, lockfile, .env. Env: `BASH_MAX_OUTPUT_LENGTH=15000`, `MAX_MCP_OUTPUT_TOKENS=10000`, `CLAUDE_NUDGE_TOKENS=80000`, `CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1`. Status line `npx --no-install ccusage statusline`. Jev: `extraKnownMarketplaces.building-with-jev = github dbreunig/building-with-jev-skill`, `enabledPlugins: { "jev@building-with-jev": true }`.
- **guard-git.mjs** (global PreToolUse Bash): blocks `gh pr merge`, `gh pr review --approve|--request-changes`, `git reset --hard`, `git clean -f`, deleting main, force push (except `--force-with-lease` to task/ or env/), and commits/pushes to main unless every file is under `.claude/Kanban/` or `.claude/PRD/` (checks paths in `git add` of the same command, staged files, and `@{u}..HEAD` on push).
- **allow-bash.mjs + bash-allowlist.json** (per-agent frontmatter hook): splits commands on `&& || ; |` and newlines (ignoring separators inside quotes; checks `$(…)` contents; strips `VAR=val` prefixes); every segment must match `common` or the role's regexes. Developer: npm/npx/node, pipeline-check, docker, curl, git working commands, push own `task/*`, gh pr create/view/diff/comment/checks/list, PR comment APIs. Reviewer: git log/diff/show/fetch, gh pr view/diff/checks/comment/list, PR comment/files APIs. DevOps: like Developer but `env/*`. Unknown role → block all.
- **quiet-bash.mjs** (global): blocks output floods (`cat` of files, raw vitest/tsc/eslint without quiet flags, unbounded `git log`, `git diff` without `--stat`/paths, unbounded `find`, `ls -R`, `tree` without depth, noisy installs) unless piped to head/tail/wc/grep, and tells Claude the quiet alternative.
- **context-nudge.mjs** (UserPromptSubmit): reads real token usage from the transcript's last usage record (fallback: size/4); past `CLAUDE_NUDGE_TOKENS` and then each +50%, once per band, tells Claude to suggest `/checkpoint` → `/clear` → `/resume`.
- **inject-history.mjs** (SessionStart, matcher `compact|resume`): prints only the latest checkpoint via `log.mjs --last checkpoint`.

## 12. Commands
- `/kickoff` (intake, stop) · `/kickoff approved` (approve + loop).
- `/next` (full loop) · `/next <ID>: <resolution>` · `/next --poll` (PR check only; used by the 30-min check). Ends with `npm run board` + ≤ 8-line summary.
- `/checkpoint`: ≤ 10 lines (module, board summary, PRs awaiting human, open questions/blockers, decisions + why, next step) saved via `npm run log -- --event checkpoint --stdin`.
- `/resume`: `log.mjs --last checkpoint`, `npm run board`, recreate the 30-min check if needed, ≤ 6-line status.

## 13. Skills
- `grill-me` (thin trigger) + `grilling` (the interview procedure) — copied verbatim from github.com/mattpocock/skills (MIT), LICENSE included.
- `to-spec` — copied and adapted (header note "Adapted from mattpocock/skills (MIT)") to write `.claude/PRD/<date-slug>/prd.md` instead of an issue tracker; PRD kept short and read sparingly; never edits Requirement/TechSpec.
- TypeSafe **Jev** skill via plugin (not vendored — its repo has no license).

## 14. Developer quick start & tooling
- `package.json` scripts: setup, dev (tsx watch), build, start, typecheck (`tsc --noEmit --pretty false`), lint (`eslint . --quiet`), format, test (`vitest run --reporter=dot --passWithNoTests`), test:smoke (placeholder until M1-T00), check (typecheck+lint+test), pipeline-check, board, log, tokens / tokens:session / tokens:blocks (ccusage). devDeps: typescript, tsx, vitest, eslint 9 + typescript-eslint strict, prettier, @types/node, ccusage.
- `tsconfig`: strict, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, NodeNext, ES2023.
- `scripts/setup.sh`: nvm use, npm ci/install, copy `.env`, **check Docker and offer to install it** (brew cask on macOS, get.docker.com on Linux), git init on main, `gh auth`, optional private repo, install Claude Code (native installer, npm fallback), next steps.
- `.vscode`: recommended extensions (anthropic.claude-code, eslint, prettier, vitest explorer, GitHub PRs, editorconfig), format on save, tasks for check and ccusage live block.
- `.gitignore`: node_modules, dist, coverage, .env* (except .env.example/.env.ci), logs, `.claude/Kanban/log.md`, `.claude/history/main.md`, settings.local.json, CLAUDE.local.md.

## 15. Token-efficiency rules (carry into every file)
- Root CLAUDE.md ≤ ~25 lines; agents `omitClaudeMd: true`.
- Agents get one task block; PRD/specs only via the PL; SOPs short and imperative.
- Board summary script instead of re-reading tasks.md; Deployed tasks archived.
- Quiet commands, capped outputs, no MCP servers for GitHub (use `gh`).
- Cheap models where safe; checkpoint → clear → resume habit enforced by the nudge.
- Logs/history/guide never read by Claude.

## 16. Verification checklist (for the recreating LLM)
- All JSON parses; `bash -n` on shell scripts.
- allow-bash: developer allowed `npm run check`, `git push -u origin HEAD`, `gh pr create`; blocked `git push origin main`, `gh pr merge`, `rm -rf`. Reviewer allowed `gh pr diff`, comment API with `$(…)` and quoted `;`; blocked `git commit`, `npm install`. Unknown role blocks everything.
- guard-git on main: allowed `git add .claude/Kanban .claude/PRD && git commit`; blocked `git add -A && git commit`, staged src commits, `gh pr merge`, `--approve`, `--force`, `reset --hard`; push to main allowed only for board-only commits.
- log.mjs writes both sinks; `--last checkpoint` returns the newest block; inject-history prints it.
- board.mjs computes frontier from `Blocked by` + Deployed/archived tasks.
- pipeline-check skips cleanly without a Dockerfile.

## 17. Earlier iterations (context only — superseded)
v1 had `.claude/context/{system,project,history,organisation}.md`, `docs/requirements/`, `/checkpoint` `/resume` `/pr`, explorer/implementer/reviewer agents. v2 added the context-nudge and quiet-bash hooks, effort setting, builder/improver agents and `/next` `/improve`. v3 (briefly) used PRD → tickets in `docs/kanban/` with per-task agent logs. The current design (this spec) replaced all of them; the token-efficiency tooling from v1–v2 is retained.
