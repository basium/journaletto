#!/usr/bin/env bash
# Black-box smoke test: GET /healthz must return 200.
# With BASE_URL set (pipeline-check) it tests that URL; otherwise it starts the compose stack itself.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ -z "${BASE_URL:-}" ]; then
  HOST_PORT="${HOST_PORT:-18080}"
  export HOST_PORT
  COMPOSE=(docker compose -f compose.yml -f compose.ci.yml)
  trap '"${COMPOSE[@]}" down -v >/dev/null 2>&1 || true' EXIT
  "${COMPOSE[@]}" up -d --build --wait
  BASE_URL="http://127.0.0.1:${HOST_PORT}"
fi

code="$(curl -sS -o /dev/null -w '%{http_code}' "$BASE_URL/healthz")"
[ "$code" = "200" ] || { echo "GET /healthz returned $code"; exit 1; }
curl -fsS "$BASE_URL/" | grep -q 'hx-get' || { echo "home page missing HTMX markup"; exit 1; }
echo "smoke ok ($BASE_URL)"
