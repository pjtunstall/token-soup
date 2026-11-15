import { createClient, type SupabaseClient } from '@supabase/supabase-js';
import { env } from '$env/dynamic/public';
import { isLocalAuth } from '$lib/auth-mode';

let client: SupabaseClient | null = null;

/** Returns a Supabase client. Only valid when PUBLIC_AUTH_MODE=supabase. */
export function getSupabase(): SupabaseClient {
	if (isLocalAuth()) {
		throw new Error('getSupabase() is not available when PUBLIC_AUTH_MODE=local');
	}
	const url = (env.PUBLIC_SUPABASE_URL ?? '').trim();
	const key = (env.PUBLIC_SUPABASE_PUBLISHABLE_KEY ?? '').trim();
	if (!url || !key) {
		throw new Error('Set PUBLIC_SUPABASE_URL and PUBLIC_SUPABASE_PUBLISHABLE_KEY in web/.env');
	}
	if (!client) {
		client = createClient(url, key);
	}
	return client;
}
