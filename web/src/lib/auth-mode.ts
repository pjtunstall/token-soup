import { env } from '$env/dynamic/public';

/** Matches api/internal/chat LocalDevToken / LocalDevUserID. */
export const LOCAL_DEV_TOKEN = 'local-dev';
export const LOCAL_DEV_USER_ID = '00000000-0000-4000-8000-000000000001';

export type AuthMode = 'local' | 'supabase';

export function getAuthMode(): AuthMode {
	const raw = (env.PUBLIC_AUTH_MODE ?? 'local').trim().toLowerCase();
	return raw === 'supabase' ? 'supabase' : 'local';
}

export function isLocalAuth(): boolean {
	return getAuthMode() === 'local';
}
