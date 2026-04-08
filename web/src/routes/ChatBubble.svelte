<script lang="ts">
	import { renderMarkdown, splitAssistantLabelPrefix } from '$lib/markdown';
	import type { ChatMessage } from '$lib/chat';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';

	let {
		msg,
		streamingHere,
		reasoningExpanded,
		onToggleReasoning
	}: {
		msg: ChatMessage;
		streamingHere: boolean;
		reasoningExpanded: boolean;
		onToggleReasoning: () => void;
	} = $props();

	const hasReasoning = $derived(
		msg.sender_type === 'bot' && !!msg.reasoning?.trim()
	);
	const showReasoning = $derived(streamingHere || hasReasoning);

	const prefix = $derived(
		msg.sender_type === 'bot'
			? splitAssistantLabelPrefix(msg.content, msg.sender_model ?? 'assistant')
			: null
	);

	function answerHasContent(): boolean {
		if (msg.sender_type !== 'bot') return false;
		if (!prefix) return msg.content.trim().length > 0;
		return prefix.rest.trim() !== '';
	}

	const showThinking = $derived(
		msg.sender_type === 'bot' && streamingHere && !answerHasContent()
	);
</script>

{#if msg.sender_type === 'user'}
	<li
		class="rounded-lg border-2 border-bubble-user-border bg-bubble-user px-3 py-2 text-sm text-foreground"
	>
		<div class="flex flex-col gap-1.5">
			<div class="font-semibold text-foreground">{msg.sender_name}</div>
			<div
				class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-headings:first:mt-0 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-strong:font-semibold prose-strong:text-foreground prose-pre:my-2"
			>
				<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes -->
				{@html renderMarkdown(msg.content)}
			</div>
		</div>
	</li>
{:else}
	<li
		class="rounded-lg border-2 border-bubble-assistant-border bg-bubble-assistant px-3 py-2 text-sm text-foreground"
	>
		{#if showReasoning}
			<div class="mb-2 flex flex-col gap-2">
				{#if !reasoningExpanded}
					<div
						class="reasoning-panel overflow-hidden rounded-md border border-border/55 bg-background/40 text-left shadow-sm"
					>
						<button
							type="button"
							class="reasoning-disclosure flex w-full items-center justify-start gap-0.5 border-b border-transparent px-2 py-1.5 text-left transition-colors hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
							onclick={onToggleReasoning}
							aria-expanded="false"
							aria-label="Show reasoning"
						>
							<span
								class="text-xs font-medium tracking-normal text-muted-foreground"
								>Reasoning</span
							>
							<ChevronDown
								class="size-3.5 shrink-0 text-muted-foreground/75"
								strokeWidth={2}
								aria-hidden="true"
							/>
						</button>
					</div>
				{:else}
					<div
						class="reasoning-panel overflow-hidden rounded-md border border-border/55 bg-background/40 text-left shadow-sm"
					>
						<button
							type="button"
							class="reasoning-disclosure flex w-full items-center justify-start gap-0.5 border-b border-border/50 px-2 py-1.5 text-left transition-colors hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
							onclick={onToggleReasoning}
							aria-expanded="true"
							aria-label="Hide reasoning"
						>
							<span
								class="text-xs font-medium tracking-normal text-muted-foreground"
								>Reasoning</span
							>
							<ChevronUp
								class="size-3.5 shrink-0 text-muted-foreground/75"
								strokeWidth={2}
								aria-hidden="true"
							/>
						</button>
						<p
							class="max-h-64 overflow-y-auto px-2 py-2 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap text-muted-foreground"
						>
							{msg.reasoning?.trim() ? msg.reasoning : 'Waiting for reasoning…'}
						</p>
					</div>
				{/if}
			</div>
		{/if}
		<div class="flex flex-col gap-1.5">
			<div
				class="flex flex-wrap items-baseline gap-x-1.5 text-sm"
				role={showThinking ? 'status' : undefined}
				aria-live={showThinking ? 'polite' : undefined}
			>
				<span
					class="font-semibold text-foreground {showThinking
						? 'assistant-model-name-pulse'
						: ''}">{msg.sender_name}</span
				>
				<span
					class="inline-flex items-baseline font-normal text-muted-foreground"
				>
					<span>({msg.sender_model ?? 'assistant'})</span>
					{#if showThinking}
						<span
							class="thinking-dots ml-1 text-muted-foreground"
							aria-hidden="true"><span>.</span><span>.</span><span>.</span></span
						>
					{/if}
				</span>
			</div>
			{#if prefix?.rest?.trim()}
				<div
					class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-headings:first:mt-0 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-strong:font-semibold prose-strong:text-foreground prose-pre:my-2"
				>
					<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes LLM HTML -->
					{@html renderMarkdown(prefix.rest)}
				</div>
			{:else if !prefix && msg.content.trim()}
				<div
					class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-headings:first:mt-0 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-strong:font-semibold prose-strong:text-foreground prose-pre:my-2"
				>
					<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes LLM HTML -->
					{@html renderMarkdown(msg.content)}
				</div>
			{/if}
		</div>
	</li>
{/if}
