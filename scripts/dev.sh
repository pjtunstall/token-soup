#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cleanup() {
	if [[ -n "${GO_PID:-}" ]] && kill -0 "$GO_PID" 2>/dev/null; then
		kill "$GO_PID" 2>/dev/null || true
	fi
	if [[ -n "${VITE_PID:-}" ]] && kill -0 "$VITE_PID" 2>/dev/null; then
		kill "$VITE_PID" 2>/dev/null || true
	fi
}

trap cleanup EXIT INT TERM

echo "Starting Go backend on :8080..."
(
	cd "$ROOT_DIR"
	go run ./cmd/ouija
) &
GO_PID=$!

echo "Starting SvelteKit dev server..."
(
	cd "$ROOT_DIR/web"
	npm run dev
) &
VITE_PID=$!

wait "$GO_PID" "$VITE_PID"
