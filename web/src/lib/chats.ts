import { PUBLIC_API_URL } from '$env/static/public';
import { MAX_TITLE_RUNES, truncateWithEllipsis, type ChatMessage } from '$lib/chat';
import { clampChatMessageForDisplay } from '$lib/display-clamp';

export type ChatSummaryRow = { id: number; title: string; lastUpdated: string };
export type ListPageParams = { limit?: number; offset?: number };

type ApiMessage = {
	sender_type: string;
	sender_name: string;
	content: string;
	sender_model?: string | null;
	reasoning?: string | null;
	role_id?: number | null;
};

type ChatDetailResponse = {
	id: number;
	title: string;
	messages: ApiMessage[];
};

function apiBase(): string {
	const raw = (PUBLIC_API_URL ?? '').trim();
	if (!raw) return '';
	return raw.replace(/\/$/, '');
}

function authHeaders(accessToken: string): HeadersInit {
	return {
		Authorization: `Bearer ${accessToken}`,
		'Content-Type': 'application/json'
	};
}

function apiMessageToChatMessage(m: ApiMessage): ChatMessage {
	if (m.sender_type === 'user') {
		return clampChatMessageForDisplay({
			sender_type: 'user',
			sender_name: m.sender_name,
			content: m.content,
			...(m.role_id != null ? { role_id: m.role_id } : {})
		});
	}
	return clampChatMessageForDisplay({
		sender_type: 'bot',
		sender_name: m.sender_name,
		content: m.content,
		sender_model: (m.sender_model ?? '').trim() || 'assistant',
		reasoning: m.reasoning ?? null,
		...(m.role_id != null ? { role_id: m.role_id } : {})
	});
}

/** GET /api/chats */
export async function fetchChats(
	accessToken: string,
	{ limit = 10, offset = 0 }: ListPageParams = {}
): Promise<ChatSummaryRow[]> {
	const base = apiBase();
	const query = new URLSearchParams({
		limit: String(limit),
		offset: String(offset)
	});
	const res = await fetch(`${base}/api/chats?${query.toString()}`, {
		method: 'GET',
		headers: authHeaders(accessToken)
	});
	if (res.status === 503) {
		throw new Error('Database is not available on the server.');
	}
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(text || `could not load chats (${res.status})`);
	}
	const rows = (await res.json()) as ChatSummaryRow[];
	return rows.map((row) => ({
		...row,
		title: truncateWithEllipsis(row.title ?? '', MAX_TITLE_RUNES)
	}));
}

/** GET /api/chats/:id */
export async function fetchChat(
	accessToken: string,
	id: number
): Promise<{ id: number; title: string; messages: ChatMessage[] }> {
	const base = apiBase();
	const res = await fetch(`${base}/api/chats/${id}`, {
		method: 'GET',
		headers: authHeaders(accessToken)
	});
	if (res.status === 503) {
		throw new Error('Database is not available on the server.');
	}
	if (res.status === 404) {
		throw new Error('Chat not found.');
	}
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(text || `could not load chat (${res.status})`);
	}
	const raw = (await res.json()) as ChatDetailResponse;
	return {
		id: raw.id,
		title: truncateWithEllipsis(raw.title ?? '', MAX_TITLE_RUNES),
		messages: raw.messages.map(apiMessageToChatMessage)
	};
}
