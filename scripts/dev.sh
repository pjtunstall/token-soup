#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${1:-local}"
API_HOST="127.0.0.1"
API_PORT="8080"
API_READY_TIMEOUT_SECS="${API_READY_TIMEOUT_SECS:-60}"

case "$MODE" in
	local)
		COMPOSE_FILES=(-f "$ROOT_DIR/docker-compose.yaml")
		;;
	supabase)
		COMPOSE_FILES=(-f "$ROOT_DIR/docker-compose.supabase.yaml")
		;;
	*)
		echo "usage: $0 [local|supabase]" >&2
		exit 1
		;;
esac

cleanup() {
	if [[ -n "${API_PID:-}" ]] && kill -0 "$API_PID" 2>/dev/null; then
		kill "$API_PID" 2>/dev/null || true
	fi
	if [[ -n "${VITE_PID:-}" ]] && kill -0 "$VITE_PID" 2>/dev/null; then
		kill "$VITE_PID" 2>/dev/null || true
	fi
	docker compose "${COMPOSE_FILES[@]}" down --remove-orphans -v >/dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM

wait_for_api() {
	local deadline=$(( SECONDS + API_READY_TIMEOUT_SECS ))
	while (( SECONDS < deadline )); do
		if (exec 3<>"/dev/tcp/${API_HOST}/${API_PORT}") 2>/dev/null; then
			exec 3<&- 3>&-
			return 0
		fi
		sleep 0.25
	done
	return 1
}

echo "Starting Go API container (mode=${MODE}) on :${API_PORT}..."
docker compose "${COMPOSE_FILES[@]}" up --build &
API_PID=$!

echo "Waiting for API on ${API_HOST}:${API_PORT}..."
if ! wait_for_api; then
	echo "API did not become ready within ${API_READY_TIMEOUT_SECS}s; aborting." >&2
	exit 1
fi
echo "API is ready."

echo "Starting SvelteKit dev server..."
(
	cd "$ROOT_DIR/web"
	npm run dev
) &
VITE_PID=$!

wait "$API_PID" "$VITE_PID"
