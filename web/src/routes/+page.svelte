<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { renderMarkdown } from '$lib/markdown';
	import { getWebSocketUrl } from '$lib/ws';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import {
		Card,
		CardContent,
		CardDescription,
		CardHeader,
		CardTitle
	} from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { ScrollArea } from '$lib/components/ui/scroll-area';

	let socket: WebSocket | null = null;
	let messageInput = $state('');
	let messages = $state<string[]>([]);
	let status = $state<'connecting' | 'connected' | 'closed' | 'error'>('connecting');
	let logContainer: HTMLDivElement | null = null;

	function appendMessage(text: string) {
		messages = [...messages, text];
		void scrollToBottom();
	}

	async function scrollToBottom() {
		await tick();
		if (!logContainer) return;
		logContainer.scrollTop = logContainer.scrollHeight;
	}

	function connect() {
		status = 'connecting';
		const ws = new WebSocket(getWebSocketUrl());
		socket = ws;

		ws.onopen = () => {
			status = 'connected';
			appendMessage('Connected.');
		};

		ws.onclose = () => {
			status = 'closed';
			appendMessage('Connection closed.');
		};

		ws.onerror = () => {
			status = 'error';
			appendMessage('Connection error.');
		};

		ws.onmessage = (event) => {
			const text = String(event.data);
			if (!text) return;
			appendMessage(text);
		};
	}

	function sendMessage() {
		if (!socket || socket.readyState !== WebSocket.OPEN || !messageInput.trim()) return;
		socket.send(messageInput);
		messageInput = '';
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		sendMessage();
	}

	onMount(connect);

	onDestroy(() => {
		socket?.close();
	});
</script>

<main class="mx-auto flex min-h-screen w-full max-w-3xl items-center p-4">
	<Card class="h-[80vh] w-full">
		<CardHeader class="flex flex-row items-center justify-between space-y-0">
			<div class="space-y-1">
				<CardTitle>Ouija Chat</CardTitle>
				<CardDescription>Human v. Human ~~~ Human v. AI ~~~ AI v. AI</CardDescription>
			</div>
			<Badge variant={status === 'connected' ? 'default' : 'secondary'}>
				{status}
			</Badge>
		</CardHeader>
		<CardContent class="flex h-[calc(100%-6rem)] flex-col gap-3">
			<ScrollArea class="min-h-0 flex-1 rounded-md border">
				<div bind:this={logContainer} class="space-y-2 p-3">
					{#if messages.length === 0}
						<p class="text-sm text-muted-foreground">No messages yet.</p>
					{:else}
						<ul class="space-y-2">
							{#each messages as msg, i (i)}
								<li class="rounded-md bg-secondary px-3 py-2 text-sm text-secondary-foreground">
									<div
										class="prose prose-sm max-w-none dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-pre:my-2"
									>
										<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes LLM HTML -->
										{@html renderMarkdown(msg)}
									</div>
								</li>
							{/each}
						</ul>
					{/if}
				</div>
			</ScrollArea>

			<form class="flex gap-2" onsubmit={handleSubmit}>
				<Input
					type="text"
					placeholder="Type a message..."
					bind:value={messageInput}
					disabled={status !== 'connected'}
				/>
				<Button type="submit" disabled={status !== 'connected' || !messageInput.trim()}>
					Send
				</Button>
			</form>
		</CardContent>
	</Card>
</main>
