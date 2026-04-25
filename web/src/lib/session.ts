import type { Session } from '@supabase/supabase-js';

export const HAD_SESSION_STORAGE_KEY = 'token-soup:hadSession';

export function getDisplayName(session: Session | null): string {
	if (!session?.user) return 'You';
	const meta = session.user.user_metadata as { display_name?: string } | undefined;
	const displayName = meta?.display_name?.trim();
	if (displayName) return displayName;
	const emailPrefix = session.user.email?.split('@')[0];
	return emailPrefix ?? 'You';
}

export function markSessionSeen(storage: Storage): void {
	try {
		storage.setItem(HAD_SESSION_STORAGE_KEY, '1');
	} catch {
		/* ignore storage failures */
	}
}

export function hasSeenSession(storage: Storage): boolean {
	try {
		return storage.getItem(HAD_SESSION_STORAGE_KEY) === '1';
	} catch {
		return false;
	}
}

/**
 * Maps load-API errors to user-facing messages, replacing a bare `unauthorized`
 * with `Session expired...` when we have prior evidence the user was signed in.
 */
export function rolesErrorMessage(err: unknown, storage: Storage): string {
	const message = err instanceof Error ? err.message : String(err);
	if (message.trim().toLowerCase() === 'unauthorized' && hasSeenSession(storage)) {
		return 'Session expired. Please sign in again.';
	}
	return message;
}
