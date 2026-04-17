import { PUBLIC_API_URL } from '$env/static/public';

/** Default interlocutor role name; matches API openrouter.RoleGuruName / future public.roles.name. */
export const DEFAULT_ROLE_NAME = 'guru' as const;

/**
 * In-app chat row aligned with public.messages (no id, chat_id, or created_at until persisted).
 * Bot `sender_model` holds the short model label from the stream (UI); the server stores the full model id.
 */
export type ChatMessage =
	| {
			sender_type: 'user';
			sender_name: string;
			content: string;
			role_id?: number | null;
	  }
	| {
			sender_type: 'bot';
			sender_name: string;
			content: string;
			sender_model: string;
			reasoning?: string | null;
			role_id?: number | null;
	  };

export type AIStreamEvent = {
	type: string;
	jobId?: number | string;
	text?: string;
	/** Assistant label from OpenRouter’s resolved model (no colon). */
	name?: string;
	/** Set on `chat_session` after the server stores a new chat row (first message). */
	chatId?: number;
};

/** Matches API chat.maxTitleRunes (strict display width). */
export const MAX_TITLE_RUNES = 48;

function isApostrophe(rs: string[], i: number): boolean {
	if (i <= 0 || i >= rs.length - 1 || rs[i] !== "'") return false;
	const prev = rs[i - 1]!;
	const next = rs[i + 1]!;
	return /\p{L}/u.test(prev) && /\p{L}/u.test(next);
}

/** First index after sentence-ending .?! outside balanced "..." or '...' (apostrophes in words skipped). */
function firstSentenceEnd(s: string): number {
	const rs = [...s];
	let inDouble = false;
	let inSingle = false;
	for (let i = 0; i < rs.length; i++) {
		const ch = rs[i];
		if (ch === '"') {
			inDouble = !inDouble;
			continue;
		}
		if (ch === "'") {
			if (isApostrophe(rs, i)) continue;
			inSingle = !inSingle;
			continue;
		}
		if (inDouble || inSingle) continue;
		if (ch === '!' || ch === '?') return i + 1;
		if (ch === '.') {
			if (i + 1 < rs.length && rs[i + 1] === '.') continue;
			return i + 1;
		}
	}
	return -1;
}

export function truncateWithEllipsis(s: string, maxRunes: number): string {
	const runes = [...s.trim()];
	if (runes.length <= maxRunes) return runes.join('');
	if (maxRunes < 4) return '...';
	return runes.slice(0, maxRunes - 3).join('') + '...';
}

/** Mirrors API FallbackTitleFromUserMessage (cancel / offline title). */
export function fallbackTitleFromUserMessage(msg: string): string {
	const trimmed = msg.trim();
	if (!trimmed) return '…';
	const end = firstSentenceEnd(trimmed);
	if (end > 0) {
		let sent = trimmed.slice(0, end).trim();
		sent = sent.replace(/,+$/u, '').trim();
		const sentRunes = [...sent];
		if (sentRunes.length <= MAX_TITLE_RUNES) return sent;
		return truncateWithEllipsis(sent, MAX_TITLE_RUNES);
	}
	return truncateWithEllipsis(trimmed, MAX_TITLE_RUNES);
}

/** Mirrors API NormalizeChatTitle for client-side defense-in-depth. */
export function normalizeChatTitle(raw: string, firstUserMessage: string): string {
	let s = raw
		.trim()
		.replace(/\r\n/g, '\n')
		.replace(/\n/g, ' ');
	s = s.replace(/\s+/g, ' ').trim();
	while (
		s.length >= 2 &&
		(s.startsWith('"') || s.startsWith("'")) &&
		s.endsWith(s[0]!)
	) {
		s = s.slice(1, -1).trim();
	}
	s = s.replace(/,+$/u, '').trim();
	if (!s) return fallbackTitleFromUserMessage(firstUserMessage);
	return truncateWithEllipsis(s, MAX_TITLE_RUNES);
}

function apiBase(): string {
	const raw = (PUBLIC_API_URL ?? '').trim();
	if (!raw) return '';
	return raw.replace(/\/$/, '');
}

async function* parseSSE(body: ReadableStream<Uint8Array>): AsyncGenerator<AIStreamEvent> {
	const reader = body.getReader();
	const decoder = new TextDecoder();
	let buffer = '';
	while (true) {
		const { done, value } = await reader.read();
		if (done) {
			if (buffer.trim()) {
				const tail = buffer.split('\n\n');
				for (const block of tail) {
					if (!block.trim()) continue;
					for (const line of block.split('\n')) {
						const trimmed = line.trim();
						if (!trimmed.startsWith('data:')) continue;
						const json = trimmed.slice(5).trimStart();
						if (!json) continue;
						yield JSON.parse(json) as AIStreamEvent;
					}
				}
			}
			break;
		}
		buffer += decoder.decode(value, { stream: true });
		const parts = buffer.split('\n\n');
		buffer = parts.pop() ?? '';
		for (const block of parts) {
			if (!block.trim()) continue;
			for (const line of block.split('\n')) {
				const trimmed = line.trim();
				if (!trimmed.startsWith('data:')) continue;
				const json = trimmed.slice(5).trimStart();
				if (!json) continue;
				yield JSON.parse(json) as AIStreamEvent;
			}
		}
	}
}

/**
 * POST /api/chat with Bearer token; streams SSE JSON events until the stream ends.
 * Abort the signal to stop the assistant (closes the request; API cancels OpenRouter).
 */
export async function postChat(
	message: string,
	senderName: string,
	accessToken: string,
	signal: AbortSignal,
	chatId: number | null,
	ephemeral: boolean,
	onEvent: (ev: AIStreamEvent) => void,
	roleId?: number | null,
	completion?: boolean,
	messageOnly?: boolean
): Promise<void> {
	const base = apiBase();
	const body: Record<string, unknown> = { message, senderName };
	if (chatId != null) {
		body.chatId = chatId;
	}
	if (roleId != null && roleId > 0) {
		body.roleId = roleId;
	}
	if (ephemeral && chatId == null) {
		body.ephemeral = true;
	}
	if (completion) {
		body.completion = true;
	}
	if (messageOnly) {
		body.messageOnly = true;
	}
	const res = await fetch(`${base}/api/chat`, {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${accessToken}`,
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(body),
		signal
	});
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(text || `chat failed (${res.status})`);
	}
	if (!res.body) {
		throw new Error('no response body');
	}
	for await (const ev of parseSSE(res.body)) {
		onEvent(ev);
	}
}
