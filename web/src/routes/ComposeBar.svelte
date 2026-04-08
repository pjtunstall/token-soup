<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import SendHorizontal from '@lucide/svelte/icons/send-horizontal';
	import Square from '@lucide/svelte/icons/square';

	let {
		messageInput = $bindable(),
		awaitingAi,
		onSubmit,
		onStop
	}: {
		messageInput: string;
		awaitingAi: boolean;
		onSubmit: () => void;
		onStop: () => void;
	} = $props();

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		onSubmit();
	}
</script>

<form class="flex gap-2" onsubmit={handleSubmit}>
	<Input
		type="text"
		placeholder="Type a message..."
		bind:value={messageInput}
		disabled={awaitingAi}
	/>
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
