<!-- Written ONLY by the Project Leader. Committed to git. Active tasks only: Deployed tasks move to archive/<date-slug>.md.
Statuses: Awaiting Approval → To Do → In Progress → Code Review → Deployed · plus HITL and Blocked (from any status; back to To Do once resolved).
Mode: AFK (agent can finish alone) | HITL (needs the human).
Task block format:

## M1-T03 · <short title>
Status: To Do · Mode: AFK · Assignee: — · Blocked by: M1-T01 · PR: —
What: <end-to-end behaviour this vertical slice delivers, user's perspective>
Acceptance:
- [ ] <criterion — becomes ≤ 2 tests>
Notes: <PRD/TechSpec knowledge the agent needs, copied in, ≤ 5 lines>
-->

# Kanban

Module: M1 · 2026-10-14-journal-entry-service

## M1-T04 · Entity extraction and relationships
Status: In Progress · Mode: AFK · Assignee: developer · Blocked by: M1-T03 · PR: —
What: Processing extracts entities and relations and stores them for graph traversal later.
Acceptance:
- [ ] Entities of the 16 JE-008 types are stored normalised and deduplicated per entry, with relationships and provenance
- [ ] Reprocessing the same entry creates no duplicate entities or relationships
Notes: Types: Projects, People, Brands, Features, Releases, Tickets, Commits, Merges, Pull Requests, Companies, Systems, Decisions, Incidents, Tasks, Concepts, Dates. LLM JSON `{type,value,relations:[{to,kind}]}`; reject unknown types. ALSO (PR #7 review): in internal/worker/steps.go ~69 an empty/blank LLM summary is stored as success and the entry goes to ready; treat blank summary as a step failure (retry/failed) with a test. Run `go fmt ./...` before opening the PR.

## M1-T05 · Semantic chunks
Status: To Do · Mode: AFK · Assignee: — · Blocked by: M1-T03 · PR: —
What: Processing splits each entry into section-level and small factual chunks, each carrying parent context.
Acceptance:
- [ ] Section chunks follow headings (no fixed token window); each chunk stores parent context (title, heading path) and is exact-text indexed
- [ ] Factual chunks come from the LLM via the provider; reprocessing replaces, never duplicates, chunks
Notes: Chunk must be understandable alone. Store granularity (section|fact), entry ref, associated entities, provenance.

## M1-T06 · Embeddings and vector index
Status: To Do · Mode: AFK · Assignee: — · Blocked by: M1-T05 · PR: —
What: Each chunk gets a locally generated embedding stored in a vector index.
Acceptance:
- [ ] Embeddings stored per chunk with provenance and nearest-neighbour query returns the expected chunk (fake embeddings)
- [ ] Reprocessing replaces vectors without duplicates
Notes: Prefer `sqlite-vec` via WASM/ncruces binding (CGO-free image). If it can't load, fall back to brute-force cosine over stored vectors and say so in the PR. Embeddings are sensitive data.

## M1-T07 · Detection rules and occurrences
Status: To Do · Mode: AFK · Assignee: — · Blocked by: M1-T03 · PR: —
What: Processing evaluates developer-configured rules and records occurrences.
Acceptance:
- [ ] Migration seeds `closed-ticket` and `merged-pr` (min confidence 0.8, version 1, enabled); matching entries get occurrences with excerpt and confidence
- [ ] Occurrences below minimum confidence are ignored; reprocessing never double-counts
Notes: Rule shape: {id,name,description,detection_prompt,enabled,minimum_confidence,version}. Multiple occurrences per entry only for distinct events. No rule UI. A rule-version change must allow reprocessing of active entries.

## M1-T08 · View and edit an entry
Status: To Do · Mode: AFK · Assignee: — · Blocked by: M1-T07, M1-T06, M1-T04 · PR: —
What: User views an entry (raw body plus tags and generated metadata around it) and edits it; derived data is regenerated.
Acceptance:
- [ ] View page shows body, user tags, status, summary and internal tags separately; created time shown in browser-local time
- [ ] Edit keeps id, filename and `created_at`, updates `updated_at`, keeps exactly one previous version under `versions/`, and re-queues processing without duplicates
Notes: Previous version path `{day}/versions/{utc_timestamp}_{entry_id}.previous.md`. Same validation as create. Atomic writes.

## M1-T09 · End-to-end integration test and Ollama adapter
Status: To Do · Mode: AFK · Assignee: — · Blocked by: M1-T08 · PR: —
What: Milestone gate test plus a real local-model adapter.
Acceptance:
- [ ] Integration test creates an entry, waits for `ready`, verifies raw file, front matter, canonical record, FTS record, chunks, entities, embeddings, provenance and rule occurrences (fake provider)
- [ ] Ollama adapter (LLM + embeddings) implements the provider interfaces, configurable endpoint/models/timeout/context/params, with unit tests against a stub HTTP server
Notes: Update requirement-to-test traceability matrix in docs for M1 requirements. No data may exist only in fixtures.

## M1-T10 · Try the Ollama adapter with real models
Status: To Do · Mode: HITL · Assignee: — · Blocked by: M1-T09 · PR: —
What: Human installs Ollama and models, sets provider `local`, saves an entry and confirms it reaches `ready`.
Acceptance:
- [ ] Human confirms a real entry reached `ready` with summary, entities, chunks and embeddings
Notes: Human must install Ollama plus an LLM and embedding model that support structured JSON output, and set config. Report any model/prompt problems with `/next M1-T10: <result>`.
