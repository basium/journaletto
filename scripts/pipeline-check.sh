#!/usr/bin/env bash
# Build check that mirrors a real build pipeline. Identical in CI and locally.
#   1. build the production image (runtime target)   2. start deps (compose.ci.yml, if any)
#   3. run the image with .env.ci                      4. wait for Docker HEALTHCHECK = healthy
#   5. run smoke tests against it (npm run test:smoke) 6. logs on failure, always tear down
# Falls back to a local build+start+health check when Docker is unavailable (warns: Docker path unverified).
set -euo pipefail
cd "$(dirname "$0")/.."

APP="${APP_NAME:-$(node -p "require('./package.json').name")}"
TAG="${APP}:check"
PORT="${PORT:-3000}"
HOST_PORT="${HOST_PORT:-18080}"
TIMEOUT="${HEALTH_TIMEOUT:-90}"
ENV_FILE=".env.ci"; [ -f "$ENV_FILE" ] || ENV_FILE=".env.example"
CID=""

say() { printf '▸ %s\n' "$1"; }
cleanup() {
  [ -n "$CID" ] && docker rm -f "$CID" >/dev/null 2>&1 || true
  [ -f compose.ci.yml ] && command -v docker >/dev/null 2>&1 && docker compose -f compose.ci.yml down -v >/dev/null 2>&1 || true
  [ -n "${LOCAL_PID:-}" ] && kill "$LOCAL_PID" 2>/dev/null || true
}
trap cleanup EXIT

if [ ! -f Dockerfile ]; then
  echo "No Dockerfile yet — the walking-skeleton task (M*-T00) creates it. Skipping."; exit 0
fi

wait_http() { # url
  local deadline=$((SECONDS + TIMEOUT))
  until curl -fsS "$1" >/dev/null 2>&1; do
    [ $SECONDS -ge $deadline ] && return 1; sleep 2
  done
}

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  say "Building $TAG (target: runtime)"
  docker build --target runtime -t "$TAG" . >/dev/null

  NET_ARGS=()
  if [ -f compose.ci.yml ]; then
    say "Starting dependencies (compose.ci.yml)"
    docker compose -f compose.ci.yml up -d --wait
    NET="$(docker compose -f compose.ci.yml ps -q | head -1 | xargs -r docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{end}}')"
    [ -n "$NET" ] && NET_ARGS=(--network "$NET")
  fi

  say "Running container"
  CID="$(docker run -d ${NET_ARGS[@]+"${NET_ARGS[@]}"} --env-file "$ENV_FILE" -e PORT="$PORT" -p "$HOST_PORT:$PORT" "$TAG")"

  say "Waiting for HEALTHCHECK (≤ ${TIMEOUT}s)"
  deadline=$((SECONDS + TIMEOUT))
  while :; do
    status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}no-healthcheck{{end}}' "$CID")"
    [ "$status" = "healthy" ] && break
    if [ "$status" = "no-healthcheck" ]; then echo "Dockerfile has no HEALTHCHECK — add one (see environment.md)."; docker logs "$CID" | tail -40; exit 1; fi
    if [ "$status" = "unhealthy" ] || [ $SECONDS -ge $deadline ]; then echo "Container not healthy ($status)"; docker logs "$CID" | tail -60; exit 1; fi
    sleep 2
  done
  BASE_URL="http://127.0.0.1:${HOST_PORT}"
else
  echo "⚠️  Docker not available — falling back to a local run. The Docker path was NOT verified. Install Docker (see scripts/setup.sh)."
  npm run build --silent
  set -a; . "./$ENV_FILE"; set +a
  PORT="$HOST_PORT" npm run start --silent > .pipeline-check.log 2>&1 &
  LOCAL_PID=$!
  if ! wait_http "http://127.0.0.1:${HOST_PORT}/health"; then echo "App not healthy"; tail -60 .pipeline-check.log; exit 1; fi
  BASE_URL="http://127.0.0.1:${HOST_PORT}"
fi

say "Smoke tests against $BASE_URL"
if ! BASE_URL="$BASE_URL" npm run test:smoke --silent; then
  [ -n "$CID" ] && docker logs "$CID" | tail -60
  exit 1
fi
say "pipeline-check passed"
