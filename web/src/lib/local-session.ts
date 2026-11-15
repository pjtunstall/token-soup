import type { Session, User } from '@supabase/supabase-js';
import { LOCAL_DEV_TOKEN, LOCAL_DEV_USER_ID } from '$lib/auth-mode';

/** Minimal Session shape so existing getDisplayName / access_token call sites work. */
export function createLocalSession(): Session {
	const user = {
		id: LOCAL_DEV_USER_ID,
		aud: 'local',
		role: 'authenticated',
		email: 'local@token-soup.dev',
		app_metadata: {},
		user_metadata: { display_name: 'User' },
		created_at: new Date(0).toISOString()
	} as User;

	return {
		access_token: LOCAL_DEV_TOKEN,
		token_type: 'bearer',
		expires_in: 60 * 60 * 24 * 365,
		expires_at: Math.floor(Date.now() / 1000) + 60 * 60 * 24 * 365,
		refresh_token: '',
		user
	};
}
