# Environment conventions

Owned by DevOps & Architect (keep it current via env PRs). Developers read it when their task touches runtime files.

## Local development
- `npm install` → `npm run dev` (hot reload) for day-to-day work.
- `docker compose up` (from `compose.yml`) runs the app plus its dependencies locally when the stack needs services.
- Config only through env vars; `.env.example` lists every variable with a safe default. Real `.env` is never committed.

## Docker
- One multi-stage `Dockerfile`: `deps` → `build` → `runtime`.
- `runtime` target: production deps only, non-root user, `NODE_ENV=production`, exposes one port, `HEALTHCHECK` calling the health endpoint (e.g. `CMD node -e "fetch('http://127.0.0.1:'+process.env.PORT+'/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"`).
- Health endpoint: `GET /health` → 200 with `{ status, version, commit }`. Non-HTTP apps: the closest equivalent the tech spec defines (`--version` for a CLI, a queue/db ping for a worker).
- The image must build and run in any CI system with plain `docker build --target runtime .` — that is the human's deployment artifact.

## Build check (`scripts/pipeline-check.sh`) — same in CI and locally
1. `docker build --target runtime -t <app>:check .`
2. If `compose.ci.yml` exists: `docker compose -f compose.ci.yml up -d --wait` (dependencies).
3. Run the image with `--env-file .env.ci`.
4. Wait until Docker reports the container **healthy** (90 s timeout).
5. `BASE_URL=… npm run test:smoke` — 1–3 black-box tests against the running container.
6. On failure print container logs; always tear down.
If Docker isn't available it falls back to `npm run build && npm run start` + health check and warns that the Docker path was not verified.

## When the build check runs
- CI: PRs from `env/*` branches, or PRs touching `Dockerfile`, `compose*.yml`, `.env.ci`, `scripts/pipeline-check.sh`, `.github/workflows/**`, `package.json`, lockfile.
- Locally: by a developer whose task touched those files; by DevOps in post-merge review under the same rule; by the PL after an env PR merges.

## CI (`.github/workflows/`)
- `ci.yml`: install → typecheck → lint → test, every PR and push to main.
- `pipeline-check.yml`: the build check under the path/branch rule above. Trivy scan and registry push present, disabled.
