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
	import SendHorizontal from '@lucide/svelte/icons/send-horizontal';
	import Square from '@lucide/svelte/icons/square';

	let socket: WebSocket | null = null;
	let messageInput = $state('');
	let messages = $state<string[]>([]);
	let status = $state<'connecting' | 'connected' | 'closed' | 'error'>('connecting');
	let awaitingAi = $state(false);
	let activeAiJobId = $state<string>('');
	let logContainer: HTMLDivElement | null = null;
	let aiBubbleIndexByJobId: Record<string, number> = {};
	let scrollRaf: number | null = null;

	function scheduleScroll() {
		if (scrollRaf !== null) return;
		scrollRaf = requestAnimationFrame(() => {
			scrollRaf = null;
			void scrollToBottom();
		});
	}

	function pushMessage(text: string) {
		const idx = messages.length;
		messages = [...messages, text];
		scheduleScroll();
		return idx;
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
			pushMessage('Connected.');
		};

		ws.onclose = () => {
			status = 'closed';
			awaitingAi = false;
			activeAiJobId = '';
			pushMessage('Connection closed.');
		};

		ws.onerror = () => {
			status = 'error';
			awaitingAi = false;
			activeAiJobId = '';
			pushMessage('Connection error.');
		};

		ws.onmessage = (event) => {
			const raw = String(event.data);
			if (!raw) return;

			// Backend batches websocket writes with '\n' separators.
			for (const part of raw.split('\n')) {
				const text = part.trimEnd();
				if (!text) continue;
				handleIncomingText(text);
			}
		};
	}

	function handleIncomingText(text: string) {
		let parsed: any;
		try {
			parsed = JSON.parse(text);
		} catch {
			pushMessage(text);
			return;
		}

		if (!parsed || typeof parsed !== 'object' || typeof parsed.type !== 'string') {
			pushMessage(text);
			return;
		}

		const jobId = parsed.jobId !== undefined ? String(parsed.jobId) : '';
		switch (parsed.type) {
			case 'ai_start': {
				const idx = pushMessage(parsed.text ?? 'openrouter: ');
				if (jobId) aiBubbleIndexByJobId[jobId] = idx;
				awaitingAi = true;
				activeAiJobId = jobId;
				return;
			}
			case 'ai_delta': {
				if (!jobId) {
					pushMessage(parsed.text ?? '');
					return;
				}
				const idx = aiBubbleIndexByJobId[jobId];
				const deltaText = parsed.text ?? '';
				if (idx === undefined) {
					const newIdx = pushMessage(deltaText);
					aiBubbleIndexByJobId[jobId] = newIdx;
					return;
				}
				const current = messages[idx] ?? '';
				messages = [...messages.slice(0, idx), current + deltaText, ...messages.slice(idx + 1)];
				scheduleScroll();
				return;
			}
			case 'ai_replace': {
				if (!jobId) {
					pushMessage(parsed.text ?? '');
					return;
				}
				const idx = aiBubbleIndexByJobId[jobId];
				const replacement = parsed.text ?? '';
				if (idx === undefined) {
					const newIdx = pushMessage(replacement);
					aiBubbleIndexByJobId[jobId] = newIdx;
					return;
				}
				messages = [...messages.slice(0, idx), replacement, ...messages.slice(idx + 1)];
				scheduleScroll();
				return;
			}
			case 'ai_end': {
				if (jobId) delete aiBubbleIndexByJobId[jobId];
				if (jobId && jobId === activeAiJobId) {
					awaitingAi = false;
					activeAiJobId = '';
				}
				return;
			}
			default: {
				// Unknown structured message: show as raw.
				pushMessage(text);
				return;
			}
		}
	}

	function sendMessage() {
		if (awaitingAi) return;
		if (!socket || socket.readyState !== WebSocket.OPEN || !messageInput.trim()) return;
		socket.send(messageInput);
		messageInput = '';
	}

	function stopMessage() {
		if (!socket || socket.readyState !== WebSocket.OPEN) return;
		socket.send(JSON.stringify({ type: 'stop' }));
		awaitingAi = false;
		activeAiJobId = '';
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
				<CardTitle>Token Soup</CardTitle>
				<CardDescription>What do I get into today?</CardDescription>
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
					disabled={status !== 'connected' || awaitingAi}
				/>
				{#if awaitingAi}
					<Button
						type="button"
						disabled={status !== 'connected'}
						on:click={stopMessage}
						aria-label="Stop"
					>
						<Square class="size-4" fill="currentColor" stroke="none" />
					</Button>
				{:else}
					<Button
						type="submit"
						disabled={status !== 'connected' || !messageInput.trim()}
						aria-label="Send"
					>
						<SendHorizontal class="size-4" />
					</Button>
				{/if}
			</form>
		</CardContent>
	</Card>
</main>
