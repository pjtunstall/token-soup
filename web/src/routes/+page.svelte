<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { renderAssistantBubble, renderMarkdown } from '$lib/markdown';
	import { postChat, type AIStreamEvent } from '$lib/chat';
	import { getSupabase } from '$lib/supabase';
	import type { Session } from '@supabase/supabase-js';
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

	let authState = $state<'loading' | 'signed_out' | 'signed_in'>('loading');
	let session = $state<Session | null>(null);
	let email = $state('');
	let password = $state('');
	let authError = $state('');
	let authMode = $state<'sign_in' | 'sign_up'>('sign_in');
	/** Shown after sign-up when email confirmation is required (no session yet). */
	let signUpMessage = $state('');
	let displayNameInput = $state('');

	type ChatBubble =
		| { role: 'user'; displayName: string; text: string }
		| { role: 'assistant'; text: string; assistantName?: string };

	let messageInput = $state('');
	let messages = $state<ChatBubble[]>([]);
	let awaitingAi = $state(false);
	let activeAiJobId = $state<string>('');
	let logContainer = $state<HTMLDivElement | null>(null);
	let aiBubbleIndexByJobId: Record<string, number> = {};
	let scrollRaf: number | null = null;
	let abortController: AbortController | null = null;

	const supabase = getSupabase();

	function getDisplayName(s: Session | null): string {
		if (!s?.user) return 'You';
		const meta = s.user.user_metadata as { display_name?: string } | undefined;
		const d = meta?.display_name?.trim();
		if (d) return d;
		const email = s.user.email?.split('@')[0];
		return email ?? 'You';
	}

	function scheduleScroll() {
		if (scrollRaf !== null) return;
		scrollRaf = requestAnimationFrame(() => {
			scrollRaf = null;
			void scrollToBottom();
		});
	}

	function pushAssistantBubble(text: string, assistantName?: string) {
		const idx = messages.length;
		const n = assistantName?.trim();
		messages = [...messages, { role: 'assistant', text, assistantName: n || undefined }];
		scheduleScroll();
		return idx;
	}

	function pushUserBubble(displayName: string, text: string) {
		const idx = messages.length;
		messages = [...messages, { role: 'user', displayName, text }];
		scheduleScroll();
		return idx;
	}

	async function scrollToBottom() {
		await tick();
		if (!logContainer) return;
		logContainer.scrollTop = logContainer.scrollHeight;
	}

	function handleStreamEvent(parsed: AIStreamEvent) {
		const jobId = parsed.jobId !== undefined ? String(parsed.jobId) : '';
		switch (parsed.type) {
			case 'ai_start': {
				const idx = pushAssistantBubble(parsed.text ?? '', parsed.name);
				if (jobId) aiBubbleIndexByJobId[jobId] = idx;
				awaitingAi = true;
				activeAiJobId = jobId;
				return;
			}
			case 'ai_delta': {
				if (!jobId) {
					pushAssistantBubble(parsed.text ?? '', parsed.name);
					return;
				}
				const idx = aiBubbleIndexByJobId[jobId];
				const deltaText = parsed.text ?? '';
				if (idx === undefined) {
					const newIdx = pushAssistantBubble(deltaText, parsed.name);
					aiBubbleIndexByJobId[jobId] = newIdx;
					return;
				}
				const current = messages[idx];
				if (!current || current.role !== 'assistant') return;
				const nm = current.assistantName ?? parsed.name;
				messages = [
					...messages.slice(0, idx),
					{ role: 'assistant', text: current.text + deltaText, assistantName: nm },
					...messages.slice(idx + 1)
				];
				scheduleScroll();
				return;
			}
			case 'ai_replace': {
				if (!jobId) {
					pushAssistantBubble(parsed.text ?? '', parsed.name);
					return;
				}
				const idx = aiBubbleIndexByJobId[jobId];
				const replacement = parsed.text ?? '';
				if (idx === undefined) {
					const newIdx = pushAssistantBubble(replacement, parsed.name);
					aiBubbleIndexByJobId[jobId] = newIdx;
					return;
				}
				const cur = messages[idx];
				const nm = cur?.role === 'assistant' ? (cur.assistantName ?? parsed.name) : parsed.name;
				messages = [
					...messages.slice(0, idx),
					{ role: 'assistant', text: replacement, assistantName: nm },
					...messages.slice(idx + 1)
				];
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
				pushAssistantBubble(JSON.stringify(parsed), parsed.name);
				return;
			}
		}
	}

	async function handleAuthSubmit(e: SubmitEvent) {
		e.preventDefault();
		authError = '';
		signUpMessage = '';

		if (authMode === 'sign_up') {
			const name = displayNameInput.trim();
			if (!name) {
				authError = 'Please enter a display name.';
				return;
			}
			const { data, error } = await supabase.auth.signUp({
				email,
				password,
				options: {
					data: {
						display_name: name
					}
				}
			});
			if (error) {
				authError = error.message;
				return;
			}
			if (data.session) {
				// Logged in immediately (e.g. email confirmation disabled in Supabase).
				return;
			}
			signUpMessage =
				'Check your email for a confirmation link. After you confirm, sign in here.';
			password = '';
			return;
		}

		const { error } = await supabase.auth.signInWithPassword({ email, password });
		if (error) authError = error.message;
	}

	async function signOut() {
		await supabase.auth.signOut();
		messages = [];
		aiBubbleIndexByJobId = {};
	}

	onMount(() => {
		void supabase.auth.getSession().then(({ data: { session: s } }) => {
			session = s;
			authState = s ? 'signed_in' : 'signed_out';
		});

		const {
			data: { subscription }
		} = supabase.auth.onAuthStateChange((_event, s) => {
			session = s;
			authState = s ? 'signed_in' : 'signed_out';
		});

		return () => subscription.unsubscribe();
	});

	async function sendMessage() {
		if (awaitingAi || !session?.access_token || !messageInput.trim()) return;
		const text = messageInput.trim();
		const who = getDisplayName(session);
		pushUserBubble(who, text);
		messageInput = '';
		awaitingAi = true;
		abortController = new AbortController();
		try {
			await postChat(text, session.access_token, abortController.signal, handleStreamEvent);
		} catch (err) {
			const name = err instanceof Error ? err.name : '';
			if (name !== 'AbortError') {
				const msg = err instanceof Error ? err.message : String(err);
				pushAssistantBubble(`Error: ${msg}`, undefined);
			}
		} finally {
			awaitingAi = false;
			activeAiJobId = '';
			abortController = null;
		}
	}

	function stopMessage() {
		abortController?.abort();
		awaitingAi = false;
		activeAiJobId = '';
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		void sendMessage();
	}

	onDestroy(() => {
		abortController?.abort();
	});
</script>

<main class="mx-auto flex min-h-screen w-full max-w-3xl items-center p-4">
	<Card class="h-[80vh] w-full">
		<CardHeader class="flex flex-row items-center justify-between space-y-0">
			<div class="space-y-1">
				<CardTitle class="text-primary">Token Soup</CardTitle>
				<CardDescription>What do I get into today?</CardDescription>
			</div>
			<div class="flex items-center gap-2">
				{#if authState === 'signed_in'}
					<Button type="button" variant="outline" size="sm" onclick={() => void signOut()}>Sign out</Button>
				{:else if authState === 'signed_out'}
					<Badge variant="secondary">signed out</Badge>
				{:else}
					<Badge variant="secondary">…</Badge>
				{/if}
			</div>
		</CardHeader>
		<CardContent class="flex h-[calc(100%-6rem)] flex-col gap-3">
			{#if authState === 'loading'}
				<p class="text-sm text-muted-foreground">Loading…</p>
			{:else if authState === 'signed_out'}
				<div class="flex max-w-sm flex-col gap-3">
					<div class="flex gap-2 text-sm">
						<button
							type="button"
							class="rounded-md px-2 py-1 underline-offset-4 hover:underline {authMode === 'sign_in'
								? 'font-medium text-foreground'
								: 'text-muted-foreground'}"
							onclick={() => {
								authMode = 'sign_in';
								authError = '';
								signUpMessage = '';
								displayNameInput = '';
							}}
						>
							Sign in
						</button>
						<span class="text-muted-foreground">·</span>
						<button
							type="button"
							class="rounded-md px-2 py-1 underline-offset-4 hover:underline {authMode === 'sign_up'
								? 'font-medium text-foreground'
								: 'text-muted-foreground'}"
							onclick={() => {
								authMode = 'sign_up';
								authError = '';
								signUpMessage = '';
							}}
						>
							Create account
						</button>
					</div>
					<form class="flex flex-col gap-2" onsubmit={handleAuthSubmit}>
						{#if authMode === 'sign_up'}
							<Input
								type="text"
								placeholder="Display name"
								bind:value={displayNameInput}
								autocomplete="nickname"
								maxlength={64}
								required
							/>
						{/if}
						<Input type="email" placeholder="Email" bind:value={email} autocomplete="email" required />
						<Input
							type="password"
							placeholder="Password"
							bind:value={password}
							autocomplete={authMode === 'sign_up' ? 'new-password' : 'current-password'}
							minlength={authMode === 'sign_up' ? 6 : undefined}
							required
						/>
						{#if authError}
							<p class="text-sm text-destructive">{authError}</p>
						{/if}
						{#if signUpMessage}
							<p class="text-sm text-muted-foreground">{signUpMessage}</p>
						{/if}
						<Button type="submit">{authMode === 'sign_up' ? 'Create account' : 'Sign in'}</Button>
					</form>
				</div>
			{:else}
				<ScrollArea class="min-h-0 flex-1 rounded-md border">
					<div bind:this={logContainer} class="space-y-2 p-3">
						{#if messages.length === 0}
							<p class="text-sm text-muted-foreground">No messages yet.</p>
						{:else}
							<ul class="space-y-2">
								{#each messages as msg, i (i)}
									{#if msg.role === 'user'}
										<li
											class="rounded-lg border-2 border-bubble-user-border bg-bubble-user px-3 py-2 text-sm text-foreground"
										>
											<p class="whitespace-pre-wrap break-words">
												<span class="font-semibold text-foreground">{msg.displayName}:</span>
												{' '}
												<span class="text-foreground/85">{msg.text}</span>
											</p>
										</li>
									{:else}
										<li
											class="rounded-lg border-2 border-bubble-assistant-border bg-bubble-assistant px-3 py-2 text-sm text-foreground"
										>
											<div
												class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-pre:my-2 prose-strong:font-semibold prose-strong:text-foreground"
											>
												<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes LLM HTML -->
												{@html renderAssistantBubble(msg.text, msg.assistantName ?? 'assistant')}
											</div>
										</li>
									{/if}
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
						disabled={awaitingAi}
					/>
					{#if awaitingAi}
						<Button type="button" onclick={stopMessage} aria-label="Stop">
							<Square class="size-4" fill="currentColor" stroke="none" />
						</Button>
					{:else}
						<Button type="submit" disabled={!messageInput.trim() || awaitingAi} aria-label="Send">
							<SendHorizontal class="size-4" />
						</Button>
					{/if}
				</form>
			{/if}
		</CardContent>
	</Card>
</main>
