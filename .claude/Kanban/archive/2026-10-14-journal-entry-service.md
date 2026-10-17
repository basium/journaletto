## M1-T00 · Walking skeleton (Go)
Status: Deployed · Mode: AFK · Assignee: developer · Blocked by: — · PR: #2
What: A running Go service with `GET /healthz` and a trivial HTMX home page, containerised, with CI green.
Acceptance:
- [ ] `npm run test:smoke` starts the compose stack and gets 200 from `/healthz`; `bash scripts/pipeline-check.sh` passes
- [ ] Old Go code and unused TS `src/`, `tsconfig*`, eslint config removed; `npm run check|test` run `go vet`/`go test`
Notes: Branch task/M1-T00-walking-skeleton from origin/feature/m1-slice1-entry-save (main lacks boilerplate); PR targets feature/m1-slice1-entry-save. Go ≥1.22, `net/http`, `html/template`, HTMX, `modernc.org/sqlite` (CGO-free). Multi-stage Dockerfile with HEALTHCHECK, `compose.yml`, `compose.ci.yml`, `.env.ci`. Keep `npm run log|board`. Adapt `.github/` workflow to run `pipeline-check.sh`. Add `config.example.yaml`/`.env.example` documenting every setting (DEV_USER_ID, data root, DB path, provider, retries, limits).

## M1-T11 · Review env PR #3 (from M1-T00)
Status: Deployed · Mode: HITL · Assignee: — · Blocked by: — · PR: #3
What: Human reviews and merges env PR #3 (VERSION/COMMIT build args in pipeline-check.sh and compose.yml; environment.md rewritten for Go).
Acceptance:
- [ ] PR #3 merged and `bash scripts/pipeline-check.sh` passes on the base branch
Notes: Base is feature/m1-slice1-entry-save. Does not block T01.

## M1-T01 · Save an entry as raw Markdown
Status: Deployed · Mode: AFK · Assignee: developer · Blocked by: M1-T00 · PR: #4
What: User opens the entry form, submits title/date/description/tags, and gets success once a raw `.md` file exists; the entry is also stored canonically in SQLite and in FTS5 with status `pending`.
Acceptance:
- [ ] Valid POST writes `data/journals/{DEV_USER_ID}/YYYY-MM-DD(UTC)/{YYYYMMDDTHHMMSSZ}_{uuidv7}.md` atomically with front matter (schema_version, id, title, journal_date, created_at, updated_at, timezone, tags, status:active) and `# title` heading
- [ ] Invalid input returns 400 `{field,message}` (JSON) / inline error (HTML); canonical row + FTS row exist; status `pending`
Notes: Limits: title 1–200; body ≤100 KB; ≤100 tags, each 1–50, lowercased, deduped; date optional default today. Timezone from browser (hidden field). UTC folder/filename; local time only for display. No media upload, links ok. Save must not depend on AI. Tag autocomplete via HTMX (existing tags). SQLite migrations, foreign keys on. Save ≤ 500 ms p95. Also: vendor htmx via go:embed (or pin with SRI) instead of loading from unpkg (follow-up from M1-T00 review). Branch from origin/feature/m1-slice1-entry-save, PR base same.


## M1-T12 · Harden entry save (review findings from PR #4)
Status: Deployed · Mode: AFK · Assignee: developer · Blocked by: M1-T01 · PR: #5
What: Saving an entry can no longer leave an orphan .md file, and malformed titles/tags cannot corrupt the raw file.
Acceptance:
- [ ] A failed or crashed DB commit leaves no orphan .md (write DB first/commit then rename, or clean up on failure); DB write lock not held across fsync
- [ ] Title with newline is rejected or collapsed (no front-matter/Markdown injection); 100-tag cap and 1–50 length checked on the raw input; unit tests in internal/entries cover all limits and boundaries
Notes: Source: reviewer comments on PR #4 (entries.go Save ~162, normalize ~71/~138). Use config MaxEntryBytes instead of the hardcoded limit if config says 100 KB (T01 left 65536 unused; spec is 100 KB). Raw file stays source of truth.
