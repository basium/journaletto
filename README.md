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

## Local LLM with Ollama (optional)
Default `docker compose up` runs the app with `PROVIDER=fake` and no Ollama. To use the local provider:
```bash
PROVIDER=local docker compose --profile local-llm up --build           # CPU
PROVIDER=local docker compose -f compose.yml -f compose.gpu.yml --profile local-llm up --build   # NVIDIA GPU
```
- The `local-llm` profile starts `ollama` (models in the `ollama-models` volume) and a one-shot `ollama-pull` that downloads `OLLAMA_LLM_MODEL` (default `llama3.1`) and `OLLAMA_EMBED_MODEL` (default `nomic-embed-text`). The app starts once Ollama is healthy and the pull has finished. Compose wires `OLLAMA_ENDPOINT=http://ollama:11434` to the app.
- First start downloads several GB (llama3.1 is about 4.7 GB); expect minutes, depending on bandwidth. Later starts reuse the volume.
- CPU works but is slow for the LLM. `compose.gpu.yml` reserves all NVIDIA GPUs (needs the NVIDIA driver and nvidia-container-toolkit on the host).
- Without Docker for Ollama: install Ollama on the host (`ollama pull llama3.1 && ollama pull nomic-embed-text`) and run the app with `PROVIDER=local` (endpoint defaults to `http://localhost:11434`). If the app itself runs in a container, set `OLLAMA_ENDPOINT=http://host.docker.internal:11434` (on Linux add `extra_hosts: ["host.docker.internal:host-gateway"]`) and do not use the profile.

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
