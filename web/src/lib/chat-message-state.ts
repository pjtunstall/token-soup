import type { ChatMessage } from '$lib/chat';

export function appendAssistantMessage(
	messages: ChatMessage[],
	senderName: string,
	text: string,
	senderModel?: string
): { messages: ChatMessage[]; index: number } {
	const index = messages.length;
	const model = senderModel?.trim() || 'assistant';
	return {
		index,
		messages: [
			...messages,
			{
				sender_type: 'bot',
				sender_name: senderName,
				sender_model: model,
				content: text
			}
		]
	};
}

export function appendUserMessage(
	messages: ChatMessage[],
	senderName: string,
	text: string
): { messages: ChatMessage[]; index: number } {
	const index = messages.length;
	return {
		index,
		messages: [...messages, { sender_type: 'user', sender_name: senderName, content: text }]
	};
}

export function applyReasoningDelta(
	messages: ChatMessage[],
	index: number,
	senderName: string,
	deltaText: string,
	senderModel?: string
): ChatMessage[] {
	const current = messages[index];
	if (!current || current.sender_type !== 'bot') return messages;
	const model = current.sender_model ?? senderModel ?? 'assistant';
	const previousReasoning = current.reasoning ?? '';
	return [
		...messages.slice(0, index),
		{
			sender_type: 'bot',
			sender_name: senderName,
			sender_model: model,
			content: current.content,
			reasoning: previousReasoning + deltaText
		},
		...messages.slice(index + 1)
	];
}

export function applyAnswerDelta(
	messages: ChatMessage[],
	index: number,
	senderName: string,
	deltaText: string,
	senderModel?: string
): ChatMessage[] {
	const current = messages[index];
	if (!current || current.sender_type !== 'bot') return messages;
	const model = current.sender_model ?? senderModel ?? 'assistant';
	return [
		...messages.slice(0, index),
		{
			sender_type: 'bot',
			sender_name: senderName,
			sender_model: model,
			content: current.content + deltaText,
			reasoning: current.reasoning
		},
		...messages.slice(index + 1)
	];
}

export function applyReplacement(
	messages: ChatMessage[],
	index: number,
	senderName: string,
	replacement: string,
	senderModel?: string
): ChatMessage[] {
	const current = messages[index];
	const model = current?.sender_type === 'bot' ? (current.sender_model ?? senderModel) : senderModel;
	const keepReasoning = current?.sender_type === 'bot' ? current.reasoning : undefined;
	return [
		...messages.slice(0, index),
		{
			sender_type: 'bot',
			sender_name: senderName,
			sender_model: model ?? 'assistant',
			content: replacement,
			reasoning: keepReasoning
		},
		...messages.slice(index + 1)
	];
}
