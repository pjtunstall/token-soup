<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { USER_MESSAGE_MAX_CHARS } from '$lib/limits';
	import HandHelping from '@lucide/svelte/icons/hand-helping';
	import SendHorizontal from '@lucide/svelte/icons/send-horizontal';
	import Square from '@lucide/svelte/icons/square';

	let {
		messageInput = $bindable(),
		awaitingAi,
		aiMode,
		onSubmit,
		onStop,
		onRequestCompletion
	}: {
		messageInput: string;
		awaitingAi: boolean;
		aiMode: boolean;
		onSubmit: () => void;
		onStop: () => void;
		onRequestCompletion: () => void;
	} = $props();

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		onSubmit();
	}
</script>

{#if aiMode}
	<div class="flex gap-2">
		<button
			type="button"
			class="flex h-9 flex-1 cursor-pointer items-center rounded-md border border-input bg-transparent px-3 py-1 text-sm text-muted-foreground shadow-xs transition-colors hover:border-ring focus-visible:border-ring focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-default disabled:opacity-50"
			disabled={awaitingAi}
			onclick={onRequestCompletion}
			aria-label="Request a message from AI"
		>
			Request a message...
		</button>
		{#if awaitingAi}
			<Button type="button" onclick={onStop} aria-label="Stop">
				<Square class="size-4" fill="currentColor" stroke="none" />
			</Button>
		{:else}
			<Button
				type="button"
				onclick={onRequestCompletion}
				aria-label="Request a message from AI"
			>
				<HandHelping class="size-4" />
			</Button>
		{/if}
	</div>
{:else}
	<form class="flex gap-2" onsubmit={handleSubmit}>
		<div class="min-w-0 flex-1">
			<Input
				type="text"
				placeholder="Type a message..."
				bind:value={messageInput}
				maxlength={USER_MESSAGE_MAX_CHARS}
				disabled={awaitingAi}
			/>
			<p class="mt-1 text-[11px] text-muted-foreground">
				{[...messageInput.trim()].length}/{USER_MESSAGE_MAX_CHARS}
			</p>
		</div>
		{#if awaitingAi}
			<Button type="button" onclick={onStop} aria-label="Stop">
				<Square class="size-4" fill="currentColor" stroke="none" />
			</Button>
		{:else}
			<Button type="submit" disabled={!messageInput.trim() || awaitingAi} aria-label="Send">
				<SendHorizontal class="size-4" />
			</Button>
		{/if}
	</form>
{/if}
