-- Minimal auth schema so supabase/schema.sql FKs and auth.uid() resolve on bare Postgres.
CREATE SCHEMA IF NOT EXISTS auth;

CREATE TABLE IF NOT EXISTS auth.users (
    id uuid PRIMARY KEY
);

CREATE OR REPLACE FUNCTION auth.uid()
RETURNS uuid
LANGUAGE sql
STABLE
AS $$
    SELECT NULL::uuid;
$$;

-- Fixed local-dev user (must match api LocalDevUserID / web local session).
INSERT INTO auth.users (id)
VALUES ('00000000-0000-4000-8000-000000000001')
ON CONFLICT (id) DO NOTHING;
