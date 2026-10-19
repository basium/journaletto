# Environment conventions

Owned by DevOps & Architect (keep it current via env PRs). Developers read it when their task touches runtime files.
Stack: Go (single static binary, embedded SQLite via modernc, server-rendered templates + htmx).

## Local development
- `go run ./cmd/journaletto` for day-to-day work; `npm run check` is the CI gate (wraps the Go checks).
- `docker compose up` (from `compose.yml`) runs the app with a named volume on `/data`. No external services: SQLite is embedded; `compose.ci.yml` is only an overlay that adds `.env.ci`.
- Optional Ollama: `docker compose --profile local-llm up` (with `PROVIDER=local`) adds `ollama` (volume `ollama-models`) and one-shot `ollama-pull` (pulls `OLLAMA_LLM_MODEL`/`OLLAMA_EMBED_MODEL`); `app` waits on them via `depends_on ... required: false`, so the default `up` (PROVIDER=fake) is unchanged. `compose.gpu.yml` is an optional NVIDIA override; `compose.ci.yml` does not involve Ollama. Non-Docker alternative: system Ollama at `localhost:11434` (`host.docker.internal` from a container). See README.
- Config only through env vars (and optional YAML, see `config.example.yaml`); `.env.example` lists every variable with a safe default. Real `.env` is never committed.

## Docker
- One multi-stage `Dockerfile`: `deps` (go mod download) -> `build` (`CGO_ENABLED=0`, `-trimpath`) -> `runtime` (alpine, non-root `app`, data in `/data` volume).
- `runtime` target: one port (3000), config by env vars only, `HEALTHCHECK` runs `journaletto -healthcheck` (probes `/healthz`; no curl in the image).
- Health endpoint: `GET /healthz` -> 200 with `{ status, version, commit }`. `VERSION`/`COMMIT` are build args (default `dev`/`unknown`); `pipeline-check.sh` and `compose.yml` pass them (from git / env). The human's own pipeline should pass `--build-arg VERSION=... --build-arg COMMIT=...`.
- The image must build and run in any CI system with plain `docker build --target runtime .` - that is the human's deployment artifact.
- Keep the `golang:<ver>-alpine` tag in the `Dockerfile` in sync with the `go` line in `go.mod`.
- Runtime must not depend on third-party CDNs. htmx is currently loaded from unpkg without SRI; vendor it (go:embed) or pin with SRI when the UI work next touches templates.

## Build check (`scripts/pipeline-check.sh`) - same in CI and locally
1. `docker build --target runtime` (with VERSION/COMMIT build args).
2. Run the image with `--env-file .env.ci` (no dependency stack).
3. Wait until Docker reports the container **healthy** (90 s timeout).
4. `BASE_URL=... npm run test:smoke` (runs `scripts/smoke.sh`) - black-box checks against the running container.
5. On failure print container logs; always tear down.
If Docker isn't available it falls back to `go build` + run + health check and warns that the Docker path was not verified.

## When the build check runs
- CI: PRs from `env/*` branches, or PRs touching `Dockerfile`, `compose*.yml`, `.env.ci`, `scripts/pipeline-check.sh`, `.github/workflows/**`, `package.json`, `go.mod`, `go.sum`.
- Locally: by a developer whose task touched those files; by DevOps in post-merge review under the same rule; by the PL after an env PR merges.

## CI (`.github/workflows/`)
- `ci.yml`: `npm run check` (Go + Node toolchains) on every PR and push to main.
- `pipeline-check.yml`: the build check under the path/branch rule above. Trivy scan and registry push present, disabled.
