# PRD — M1 · Standalone Journal Entry Service (Super MVP)

Source: `techspec.snapshot.md` §3.1, §3.2, §3.5 (rules only), §9.1, §10. `requirement.snapshot.md` is empty.

## Goal
A standalone service where a user saves a Markdown journal entry as a raw `.md` file (source of truth), and the entry is then fully processed in the background so later milestones (search, recall, statistics) need no new ingestion.

## Decisions (from grilling)
- Stack: **Go** (≥1.22), `net/http` + `html/template` + **HTMX** (server-rendered, no SPA), pure-Go SQLite `modernc.org/sqlite` (CGO-free), `sqlite-vec` via WASM binding (fallback: brute-force cosine, recorded in PR).
- Previous Go code from commit 1840bc3 is discarded. T00 re-initialises the module, removes the unused TS `src/`, `tsconfig*`, eslint config; dev tooling (`npm run log/board`) stays in `package.json`; `check`/`test`/`test:smoke` are rewired to Go.
- No auth: one immutable `DEV_USER_ID` (UUID) from config. Layout `data/journals/{user_id}/YYYY-MM-DD/{YYYYMMDDTHHMMSSZ}_{uuidv7}.md` — **UTC** folder and filename; browser local time for display only; `timezone` in front matter.
- Validation: title required 1–200 chars; description ≤ 100 KB; ≤ 100 tags, each 1–50 chars, lowercased, de-duplicated; date optional `YYYY-MM-DD`, default today. Errors: 400 `{field,message}` JSON, or inline form error for HTML.
- Save returns success once the raw file is atomically written; status `pending` → `processing` → `ready` | `failed`. Worker in-process, SQLite job table, 3 attempts with exponential backoff, concurrency configurable (default 1). `POST /entries/{id}/resync` and a startup sweep re-queue `pending`/`failed`.
- Models behind a provider interface: `fake` (deterministic, used by all automated tests), `local` (Ollama for LLM + embeddings), `external` (not built in M1). Provenance (provider, model, prompt version, timestamp) stored with each generated artefact.
- Entities: JSON `{type,value,relations:[{to,kind}]}`, 16 types from JE-008, normalised and de-duplicated per entry; stored in SQLite entity/relationship tables.
- Detection rules: seeded by migration — `closed-ticket` ("Tickets closed") and `merged-pr` ("Pull requests merged"), min confidence 0.8, version 1. Occurrences idempotent, store excerpt + confidence.
- CI: GitHub Actions runs `scripts/pipeline-check.sh` (adapted to Go).

## Out of scope
Recall, search page, statistics page, delete/restore, rule-management UI, auth, PDF export, external providers (M2–M5).

## Completion criterion
An automated integration test creates an entry, waits for `ready`, and verifies: raw file, front matter, canonical record, FTS record, chunks, entities, embeddings, model provenance, rule occurrences. Real-model check against local Ollama is a HITL task.
