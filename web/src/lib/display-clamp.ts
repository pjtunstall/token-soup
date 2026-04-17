import type { ChatMessage } from '$lib/chat';
import type { RoleRow } from '$lib/roles';
import {
	ROLE_DESCRIPTION_MAX_CHARS,
	ROLE_NAME_MAX_CHARS,
	USER_MESSAGE_MAX_CHARS
} from '$lib/limits';

const MAX_SENDER_MODEL_CHARS = 256;

export function clampToRunes(s: string, maxRunes: number): string {
	if (maxRunes < 1) return '';
	const runes = [...s];
	if (runes.length <= maxRunes) return s;
	return runes.slice(0, maxRunes).join('');
}

export function clampRoleForDisplay(r: RoleRow): RoleRow {
	return {
		...r,
		name: clampToRunes(r.name ?? '', ROLE_NAME_MAX_CHARS),
		description: r.description == null ? null : clampToRunes(r.description, ROLE_DESCRIPTION_MAX_CHARS)
	};
}

export function clampChatMessageForDisplay(msg: ChatMessage): ChatMessage {
	if (msg.sender_type === 'user') {
		return {
			...msg,
			sender_name: clampToRunes(msg.sender_name ?? '', ROLE_NAME_MAX_CHARS),
			content: clampToRunes(msg.content ?? '', USER_MESSAGE_MAX_CHARS)
		};
	}
	return {
		...msg,
		sender_name: clampToRunes(msg.sender_name ?? '', ROLE_NAME_MAX_CHARS),
		content: clampToRunes(msg.content ?? '', USER_MESSAGE_MAX_CHARS),
		reasoning: msg.reasoning == null ? null : clampToRunes(msg.reasoning, USER_MESSAGE_MAX_CHARS),
		sender_model: clampToRunes(msg.sender_model ?? 'assistant', MAX_SENDER_MODEL_CHARS) || 'assistant'
	};
}
