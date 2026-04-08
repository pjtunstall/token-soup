<script lang="ts">
	import type { ChatMessage } from '$lib/chat';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import ChatBubble from './ChatBubble.svelte';
	import ComposeBar from './ComposeBar.svelte';

	let {
		messages,
		awaitingAi,
		messageInput = $bindable(),
		logContainer = $bindable(),
		activeStreamingIndex,
		reasoningExpandedByIndex,
		onToggleReasoning,
		onSend,
		onStop
	}: {
		messages: ChatMessage[];
		awaitingAi: boolean;
		messageInput: string;
		logContainer: HTMLDivElement | null;
		activeStreamingIndex: number | undefined;
		reasoningExpandedByIndex: Record<number, boolean>;
		onToggleReasoning: (index: number) => void;
		onSend: () => void;
		onStop: () => void;
	} = $props();
</script>

<ScrollArea class="min-h-0 flex-1 rounded-md border border-input bg-field">
	<div bind:this={logContainer} class="min-h-full space-y-2 p-3">
		{#if messages.length === 0}
			<p class="text-sm text-muted-foreground">No messages yet.</p>
		{:else}
			<ul class="space-y-2">
				{#each messages as msg, i (i)}
					<ChatBubble
						{msg}
						streamingHere={awaitingAi && activeStreamingIndex === i}
						reasoningExpanded={!!reasoningExpandedByIndex[i]}
						onToggleReasoning={() => onToggleReasoning(i)}
					/>
				{/each}
			</ul>
		{/if}
	</div>
</ScrollArea>

<div class="mt-3">
	<ComposeBar bind:messageInput {awaitingAi} onSubmit={onSend} {onStop} />
</div>
