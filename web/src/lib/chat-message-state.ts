import type { AIStreamEvent, ChatMessage } from '$lib/chat';

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
	const model =
		current?.sender_type === 'bot' ? (current.sender_model ?? senderModel) : senderModel;
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

export type StreamReducerState = {
	messages: ChatMessage[];
	aiBubbleIndexByJobId: Record<string, number>;
	reasoningExpandedByIndex: Record<number, boolean>;
};

export type StreamLifecycle = { kind: 'started' | 'ended'; jobId: string };

export type StreamReducerResult = StreamReducerState & {
	lifecycle?: StreamLifecycle;
};

/**
 * Pure reducer for AI stream events. Returns the next message-log state plus an
 * optional lifecycle hint; the caller owns `awaitingAi` / `activeAiJobId` and
 * decides what to do with the hint. Returned object identities only change when
 * the corresponding field changes, so callers can use `===` to detect mutations
 * (e.g. to schedule a scroll only when `messages` actually changed).
 */
export function reduceStreamEvent(
	state: StreamReducerState,
	event: AIStreamEvent,
	assistantName: string
): StreamReducerResult {
	const jobId = event.jobId !== undefined ? String(event.jobId) : '';
	switch (event.type) {
		case 'ai_start': {
			const appended = appendAssistantMessage(
				state.messages,
				assistantName,
				event.text ?? '',
				event.name
			);
			const aiBubbleIndexByJobId = jobId
				? { ...state.aiBubbleIndexByJobId, [jobId]: appended.index }
				: state.aiBubbleIndexByJobId;
			return {
				messages: appended.messages,
				aiBubbleIndexByJobId,
				reasoningExpandedByIndex: state.reasoningExpandedByIndex,
				lifecycle: { kind: 'started', jobId }
			};
		}
		case 'ai_reasoning_delta': {
			const deltaText = event.text ?? '';
			if (!deltaText || !jobId) return state;
			let idx = state.aiBubbleIndexByJobId[jobId];
			let messages = state.messages;
			let aiBubbleIndexByJobId = state.aiBubbleIndexByJobId;
			let lifecycle: StreamLifecycle | undefined;
			if (idx === undefined) {
				const appended = appendAssistantMessage(
					messages,
					assistantName,
					event.name ? `${event.name}: ` : '',
					event.name
				);
				idx = appended.index;
				messages = appended.messages;
				aiBubbleIndexByJobId = { ...aiBubbleIndexByJobId, [jobId]: idx };
				lifecycle = { kind: 'started', jobId };
			}
			messages = applyReasoningDelta(messages, idx, assistantName, deltaText, event.name);
			return {
				messages,
				aiBubbleIndexByJobId,
				reasoningExpandedByIndex: state.reasoningExpandedByIndex,
				lifecycle
			};
		}
		case 'ai_delta': {
			const deltaText = event.text ?? '';
			if (!jobId) {
				const appended = appendAssistantMessage(
					state.messages,
					assistantName,
					deltaText,
					event.name
				);
				return {
					messages: appended.messages,
					aiBubbleIndexByJobId: state.aiBubbleIndexByJobId,
					reasoningExpandedByIndex: state.reasoningExpandedByIndex
				};
			}
			const idx = state.aiBubbleIndexByJobId[jobId];
			if (idx === undefined) {
				const appended = appendAssistantMessage(
					state.messages,
					assistantName,
					deltaText,
					event.name
				);
				return {
					messages: appended.messages,
					aiBubbleIndexByJobId: {
						...state.aiBubbleIndexByJobId,
						[jobId]: appended.index
					},
					reasoningExpandedByIndex: state.reasoningExpandedByIndex
				};
			}
			const reasoningExpandedByIndex =
				deltaText.trim() !== ''
					? { ...state.reasoningExpandedByIndex, [idx]: false }
					: state.reasoningExpandedByIndex;
			const messages = applyAnswerDelta(state.messages, idx, assistantName, deltaText, event.name);
			return {
				messages,
				aiBubbleIndexByJobId: state.aiBubbleIndexByJobId,
				reasoningExpandedByIndex
			};
		}
		case 'ai_replace': {
			const replacement = event.text ?? '';
			if (!jobId) {
				const appended = appendAssistantMessage(
					state.messages,
					assistantName,
					replacement,
					event.name
				);
				return {
					messages: appended.messages,
					aiBubbleIndexByJobId: state.aiBubbleIndexByJobId,
					reasoningExpandedByIndex: state.reasoningExpandedByIndex
				};
			}
			const idx = state.aiBubbleIndexByJobId[jobId];
			if (idx === undefined) {
				const appended = appendAssistantMessage(
					state.messages,
					assistantName,
					replacement,
					event.name
				);
				return {
					messages: appended.messages,
					aiBubbleIndexByJobId: {
						...state.aiBubbleIndexByJobId,
						[jobId]: appended.index
					},
					reasoningExpandedByIndex: state.reasoningExpandedByIndex
				};
			}
			const messages = applyReplacement(
				state.messages,
				idx,
				assistantName,
				replacement,
				event.name
			);
			return {
				messages,
				aiBubbleIndexByJobId: state.aiBubbleIndexByJobId,
				reasoningExpandedByIndex: state.reasoningExpandedByIndex
			};
		}
		case 'ai_end': {
			let aiBubbleIndexByJobId = state.aiBubbleIndexByJobId;
			if (jobId && jobId in aiBubbleIndexByJobId) {
				aiBubbleIndexByJobId = { ...aiBubbleIndexByJobId };
				delete aiBubbleIndexByJobId[jobId];
			}
			return {
				messages: state.messages,
				aiBubbleIndexByJobId,
				reasoningExpandedByIndex: state.reasoningExpandedByIndex,
				lifecycle: { kind: 'ended', jobId }
			};
		}
		default:
			return state;
	}
}
