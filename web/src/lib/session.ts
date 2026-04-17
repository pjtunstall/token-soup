import type { Session } from '@supabase/supabase-js';

export function getDisplayName(session: Session | null): string {
	if (!session?.user) return 'You';
	const meta = session.user.user_metadata as { display_name?: string } | undefined;
	const displayName = meta?.display_name?.trim();
	if (displayName) return displayName;
	const emailPrefix = session.user.email?.split('@')[0];
	return emailPrefix ?? 'You';
}
