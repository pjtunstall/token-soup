This project’s **source of truth** for the app schema is **`../schema.sql`**.

- **Apply / refresh the database** (drops and recreates the entire `public` schema, then runs DDL + grants): from the repo root run `npm run db:apply-schema` (or `./scripts/apply-schema.sh`).
- **`supabase db push`** tracks **versioned migrations** and runs each file **once**. It will **not** re-run an old migration when you edit SQL, so it is **not** used here to sync the full schema after every edit.

If you use the Supabase Dashboard migration UI, prefer running the same SQL as in `schema.sql` or use the apply script against your `DATABASE_URL`.
