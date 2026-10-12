# 0001 — Milestone 1, Slice 1: Entry-Save Endpoint

**Status:** Implemented
**Scope:** `POST /api/entries` — validation and atomic raw Markdown persistence only. No SQLite, no LLM, no indexing.

## Context

The spec (`docs/requirements.md`) leaves several details underspecified by design (§8 "Explicitly Unspecified": implementation language/framework, API contracts, and others). Building the first vertical slice required resolving a number of these gaps. This document records each resolution so it is traceable and can be revisited without being mistaken for an accidental deviation from the spec.

## Decisions

- **API-only, no frontend.** JE-001 describes form fields; this slice implements their JSON-API equivalent. UI design is explicitly unspecified (§8), and the task at hand is backend persistence.

- **Module path**: `github.com/basium/journaletto`, matching the repo's git remote.

- **HTTP layer**: stdlib `net/http` with Go 1.22's method+path `ServeMux` (`"POST /api/entries"`). No third-party router — a single route doesn't justify one, and it keeps the dependency surface minimal per §10's zero-cost/replaceable-components principle.

- **Libraries**: `github.com/google/uuid` v1.6.0 (`uuid.NewV7()` for JE-004's `id`), `gopkg.in/yaml.v3` for front-matter marshaling. No validation framework — the rules are simple enough to hand-roll.

- **100 KB limit (§7 Performance) applies to the fully rendered file** (front matter + heading + body), not just the description field. Rationale: the entry *is* the file; measuring only the description would let a user smuggle a much larger file via an oversized tag list or title. Enforced in `journal.Save`, before any directory is created or file written.

- **Tags accepted as a JSON array of strings**, not a comma-separated string. The comma-separated-with-autocomplete description in JE-001 is a UI widget concern; this slice's wire format is JSON. Tags are trimmed, empty entries dropped, exact duplicates removed (first-seen order kept), and case is preserved (not forced to lowercase — not specified). Exceeding the 100-tag cap (§7 Performance) is a validation error, never silent truncation, per the project rule against silently altering user input.

- **`journal_date` default resolution.** JE-001 says the field "defaults to today" but doesn't say whose "today." This implementation computes "today" in the client's submitted IANA `timezone` (the same field JE-004 already requires), not naive server-UTC-today. This avoids assigning the wrong calendar day to a user journaling near midnight in a non-UTC timezone, and avoids introducing a second, redundant "local date" field.

- **`timezone` validation**: required; validated via `time.LoadLocation`. The value `"Local"` is explicitly rejected — it resolves to the *server's* local zone, which is not a meaningful, portable value to store in a per-user journal entry.

- **JE-005 content-restriction scope**: this slice rejects only standard Markdown image syntax (`![alt](url)`) via regex. Deeper media-embed detection (raw HTML `<img>`/`<video>` tags, iframes, base64 data URIs) is out of scope here. This is a deliberate, narrow interpretation: the MVP has no upload endpoint at all, which structurally prevents real media upload/embedding regardless of what the Markdown text contains.

- **JE-012 processing-status handling.** The response includes `"status": "pending"`, but nothing durable tracks it yet — there is no database in this slice. It is a response-only signal meaning "the raw file is safely persisted; the processing pipeline (enrichment, indexing, retry, resync) does not exist yet." Full `pending`/`processing`/`ready`/`failed` tracking requires the SQLite job table introduced in a later Milestone 1 slice. This is called out in code comments on the handler so it is never mistaken for tracked state.

- **UTC daily folder vs. `journal_date` are kept distinct.** JE-003's daily folder is based on the UTC instant of submission; the front matter's `journal_date` is the day the entry represents. These can legitimately differ (e.g., backdating an entry), so the implementation never conflates them.

- **Durability**: writes use temp-file-in-same-directory + `fsync` + `rename` (atomic on the same filesystem). Fsync of the *containing directory* (needed to survive a hard power loss exactly at the rename boundary) is not implemented — it adds platform-specific complexity (no directory file descriptors on Windows) for marginal benefit at this development stage. Documented here as a known limitation, not a silent gap.

## Explicitly out of scope for this slice

SQLite/canonical DB row (JE-006), FTS5 indexing, LLM enrichment (JE-007), entity extraction (JE-008), viewing endpoint (JE-009), editing/versioning (JE-010), soft delete (JE-011), durable processing-status tracking, detection rules/statistics (DET-*), authentication, frontend.
