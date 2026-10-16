## M1-T00 · Walking skeleton (Go)
Status: Deployed · Mode: AFK · Assignee: developer · Blocked by: — · PR: #2
What: A running Go service with `GET /healthz` and a trivial HTMX home page, containerised, with CI green.
Acceptance:
- [ ] `npm run test:smoke` starts the compose stack and gets 200 from `/healthz`; `bash scripts/pipeline-check.sh` passes
- [ ] Old Go code and unused TS `src/`, `tsconfig*`, eslint config removed; `npm run check|test` run `go vet`/`go test`
Notes: Branch task/M1-T00-walking-skeleton from origin/feature/m1-slice1-entry-save (main lacks boilerplate); PR targets feature/m1-slice1-entry-save. Go ≥1.22, `net/http`, `html/template`, HTMX, `modernc.org/sqlite` (CGO-free). Multi-stage Dockerfile with HEALTHCHECK, `compose.yml`, `compose.ci.yml`, `.env.ci`. Keep `npm run log|board`. Adapt `.github/` workflow to run `pipeline-check.sh`. Add `config.example.yaml`/`.env.example` documenting every setting (DEV_USER_ID, data root, DB path, provider, retries, limits).

