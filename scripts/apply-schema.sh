#!/usr/bin/env bash
# Apply supabase/schema.sql to DATABASE_URL (rebuilds `public`; data in that schema is lost).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCHEMA="$ROOT/supabase/schema.sql"

if [[ ! -f "$SCHEMA" ]]; then
  echo "Missing $SCHEMA" >&2
  exit 1
fi

if [[ -f "$ROOT/backend/.env" ]]; then
  set -a
  # shellcheck source=/dev/null
  source "$ROOT/backend/.env"
  set +a
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "DATABASE_URL is not set. Put it in backend/.env or export it in the shell." >&2
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found. Install the PostgreSQL client (e.g. sudo apt install postgresql-client)." >&2
  exit 1
fi

echo "Applying $SCHEMA ..."
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$SCHEMA"
echo "Done."
