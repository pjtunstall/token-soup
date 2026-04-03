import { PUBLIC_API_URL } from '$env/static/public';

export type AIStreamEvent = {
	type: string;
	jobId?: number | string;
	text?: string;
	/** Assistant label from OpenRouter’s resolved model (no colon). */
	name?: string;
};

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
 * Abort the signal to stop the assistant (closes the request; backend cancels OpenRouter).
 */
export async function postChat(
	message: string,
	accessToken: string,
	signal: AbortSignal,
	onEvent: (ev: AIStreamEvent) => void
): Promise<void> {
	const base = apiBase();
	const res = await fetch(`${base}/api/chat`, {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${accessToken}`,
			'Content-Type': 'application/json'
		},
		body: JSON.stringify({ message }),
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
