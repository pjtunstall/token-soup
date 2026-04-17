<script lang="ts">
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import type { ChatSummaryRow } from '$lib/chats';

	let {
		chatRows,
		chatsLoading,
		chatsLoadingMore,
		chatsLoadError,
		chatsHasMore,
		activeChatId,
		onNewChat,
		onOpenChat,
		onLoadMore
	}: {
		chatRows: ChatSummaryRow[];
		chatsLoading: boolean;
		chatsLoadingMore: boolean;
		chatsLoadError: string;
		chatsHasMore: boolean;
		activeChatId: number | null;
		onNewChat: () => void;
		onOpenChat: (c: ChatSummaryRow) => void;
		onLoadMore: () => void;
	} = $props();

	let viewportRef = $state<HTMLElement | null>(null);
	const LOAD_MORE_PX = 240;

	function formatChatListDate(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	}

	function maybeLoadMore() {
		if (!viewportRef || chatsLoading || chatsLoadingMore || !chatsHasMore) return;
		const remaining = viewportRef.scrollHeight - viewportRef.scrollTop - viewportRef.clientHeight;
		if (remaining <= LOAD_MORE_PX) onLoadMore();
	}

	$effect(() => {
		if (!viewportRef) return;
		const onScroll = () => maybeLoadMore();
		viewportRef.addEventListener('scroll', onScroll, { passive: true });
		return () => viewportRef?.removeEventListener('scroll', onScroll);
	});

	$effect(() => {
		chatRows.length;
		maybeLoadMore();
	});
</script>

<ScrollArea
	class="min-h-0 flex-1 rounded-md border border-input bg-field"
	bind:viewportRef
>
	<div class="min-h-full space-y-2 p-3">
		{#if chatsLoading}
			<p class="text-sm text-muted-foreground">Loading chats…</p>
		{/if}
		{#if chatsLoadError}
			<p class="text-sm text-destructive">{chatsLoadError}</p>
		{/if}
		<ul class="space-y-1.5">
			<li>
				<button
					type="button"
					class="menu-action-btn w-full rounded-md border border-border bg-background/80 px-3 py-2.5 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
					onclick={onNewChat}
				>
					New chat
				</button>
			</li>
			{#each chatRows as c (c.id)}
				{@const dateLabel = formatChatListDate(c.lastUpdated)}
				<li>
					<button
						type="button"
						class="menu-action-btn w-full rounded-md border px-3 py-2.5 text-left text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline {activeChatId ===
						c.id
							? 'border-primary/50 bg-muted/40'
							: 'border-border bg-background/40 hover:bg-muted/50'}"
						onclick={() => onOpenChat(c)}
					>
						<div class="font-medium text-foreground">{c.title.trim() || 'Untitled'}</div>
						{#if dateLabel}
							<div class="mt-0.5 text-xs text-muted-foreground">{dateLabel}</div>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
		{#if !chatsLoading && !chatsLoadError && chatRows.length === 0}
			<p class="text-sm text-muted-foreground">No saved chats yet.</p>
		{/if}
		{#if chatsLoadingMore}
			<p class="text-sm text-muted-foreground">Loading more chats…</p>
		{/if}
	</div>
</ScrollArea>
