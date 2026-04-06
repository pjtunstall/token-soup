<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { renderMarkdown, splitAssistantLabelPrefix } from '$lib/markdown';
	import {
		postChat,
		DEFAULT_ROLE_NAME,
		normalizeChatTitle,
		fallbackTitleFromUserMessage,
		type AIStreamEvent,
		type ChatMessage
	} from '$lib/chat';
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
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';
	import Moon from '@lucide/svelte/icons/moon';
	import Sun from '@lucide/svelte/icons/sun';
	import { toggleTheme as applyToggleTheme } from '$lib/theme';

	let authState = $state<'loading' | 'signed_out' | 'signed_in'>('loading');
	let session = $state<Session | null>(null);
	let email = $state('');
	let password = $state('');
	let authError = $state('');
	let authMode = $state<'sign_in' | 'sign_up'>('sign_in');
	/** Shown after sign-up when email confirmation is required (no session yet). */
	let signUpMessage = $state('');
	let displayNameInput = $state('');

	let messageInput = $state('');
	let messages = $state<ChatMessage[]>([]);
	let awaitingAi = $state(false);
	let activeAiJobId = $state<string>('');
	let logContainer = $state<HTMLDivElement | null>(null);
	let aiBubbleIndexByJobId: Record<string, number> = {};
	/** Per-message index: expanded reasoning panel (click-to-reveal). */
	let reasoningExpandedByIndex = $state<Record<number, boolean>>({});
	let scrollRaf: number | null = null;
	let abortController: AbortController | null = null;

	const defaultSessionSubtitle = 'What do I get into today?';
	let sessionSubtitle = $state(defaultSessionSubtitle);

	const supabase = getSupabase();

	function assistantStreamContext(i: number, msg: ChatMessage) {
		if (msg.sender_type !== 'bot') {
			return { streamingHere: false, hasReasoning: false };
		}
		const activeIdx =
			activeAiJobId !== '' ? aiBubbleIndexByJobId[activeAiJobId] : undefined;
		const streamingHere = awaitingAi && activeIdx === i;
		const hasReasoning = !!msg.reasoning?.trim();
		return { streamingHere, hasReasoning };
	}

	function assistantAnswerHasContent(msg: ChatMessage): boolean {
		if (msg.sender_type !== 'bot') return false;
		const split = splitAssistantLabelPrefix(msg.content, msg.sender_model ?? 'assistant');
		if (!split) return msg.content.trim().length > 0;
		return split.rest.trim() !== '';
	}

	/** Model label pulses + ellipsis until the first answer tokens arrive. */
	function showAssistantModelThinking(i: number, msg: ChatMessage): boolean {
		if (msg.sender_type !== 'bot') return false;
		if (!assistantStreamContext(i, msg).streamingHere) return false;
		return !assistantAnswerHasContent(msg);
	}

	/** Separate control to open reasoning (while streaming or after stop, if any). */
	function showReasoningControl(i: number, msg: ChatMessage): boolean {
		const { streamingHere, hasReasoning } = assistantStreamContext(i, msg);
		return streamingHere || hasReasoning;
	}

	function toggleReasoningExpanded(i: number) {
		const next = !reasoningExpandedByIndex[i];
		reasoningExpandedByIndex = { ...reasoningExpandedByIndex, [i]: next };
	}

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

	function pushAssistantBubble(text: string, senderModel?: string) {
		const idx = messages.length;
		const model = senderModel?.trim() || 'assistant';
		messages = [
			...messages,
			{
				sender_type: 'bot',
				sender_name: DEFAULT_ROLE_NAME,
				sender_model: model,
				content: text
			}
		];
		scheduleScroll();
		return idx;
	}

	function pushUserBubble(senderName: string, text: string) {
		const idx = messages.length;
		messages = [...messages, { sender_type: 'user', sender_name: senderName, content: text }];
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
			case 'ai_reasoning_delta': {
				const deltaText = parsed.text ?? '';
				if (!deltaText) return;
				if (!jobId) return;
				let idx = aiBubbleIndexByJobId[jobId];
				if (idx === undefined) {
					idx = pushAssistantBubble(parsed.name ? `${parsed.name}: ` : '', parsed.name);
					aiBubbleIndexByJobId[jobId] = idx;
					awaitingAi = true;
					activeAiJobId = jobId;
				}
				const current = messages[idx];
				if (!current || current.sender_type !== 'bot') return;
				const nm = current.sender_model ?? parsed.name;
				const prevReasoning = current.reasoning ?? '';
				messages = [
					...messages.slice(0, idx),
					{
						sender_type: 'bot',
						sender_name: DEFAULT_ROLE_NAME,
						sender_model: nm ?? 'assistant',
						content: current.content,
						reasoning: prevReasoning + deltaText
					},
					...messages.slice(idx + 1)
				];
				scheduleScroll();
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
				if (!current || current.sender_type !== 'bot') return;
				const nm = current.sender_model ?? parsed.name;
				const answerStarted = deltaText.trim() !== '';
				if (answerStarted) {
					reasoningExpandedByIndex = { ...reasoningExpandedByIndex, [idx]: false };
				}
				messages = [
					...messages.slice(0, idx),
					{
						sender_type: 'bot',
						sender_name: DEFAULT_ROLE_NAME,
						sender_model: nm ?? 'assistant',
						content: current.content + deltaText,
						reasoning: current.reasoning
					},
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
				const nm =
					cur?.sender_type === 'bot' ? (cur.sender_model ?? parsed.name) : parsed.name;
				const keepReasoning = cur?.sender_type === 'bot' ? cur.reasoning : undefined;
				messages = [
					...messages.slice(0, idx),
					{
						sender_type: 'bot',
						sender_name: DEFAULT_ROLE_NAME,
						sender_model: nm ?? 'assistant',
						content: replacement,
						reasoning: keepReasoning
					},
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
			default:
				return;
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
		reasoningExpandedByIndex = {};
		sessionSubtitle = defaultSessionSubtitle;
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
		const wasEmpty = messages.length === 0;
		const text = messageInput.trim();
		const who = getDisplayName(session);
		pushUserBubble(who, text);
		messageInput = '';
		awaitingAi = true;
		abortController = new AbortController();
		let titleReceivedThisSend = false;
		function onStreamEvent(parsed: AIStreamEvent) {
			if (parsed.type === 'chat_title') {
				titleReceivedThisSend = true;
				sessionSubtitle = normalizeChatTitle(parsed.text ?? '', text);
				return;
			}
			handleStreamEvent(parsed);
		}
		try {
			await postChat(text, who, session.access_token, abortController.signal, onStreamEvent);
		} catch (err) {
			const name = err instanceof Error ? err.name : '';
			if (name !== 'AbortError') {
				const msg = err instanceof Error ? err.message : String(err);
				pushAssistantBubble(`Error: ${msg}`, undefined);
			}
		} finally {
			if (wasEmpty && !titleReceivedThisSend) {
				sessionSubtitle = fallbackTitleFromUserMessage(text);
			}
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

	function handleToggleTheme() {
		applyToggleTheme();
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
				<CardDescription>{sessionSubtitle}</CardDescription>
			</div>
			<div class="flex items-center gap-2">
				<Button
					type="button"
					variant="outline"
					size="icon-sm"
					class="shrink-0"
					onclick={handleToggleTheme}
					aria-label="Toggle color theme"
					title="Light / dark"
				>
					<Sun class="hidden size-4 dark:block" />
					<Moon class="size-4 dark:hidden" />
				</Button>
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
				<ScrollArea class="min-h-0 flex-1 rounded-md border border-input bg-field">
					<div bind:this={logContainer} class="min-h-full space-y-2 p-3">
						{#if messages.length === 0}
							<p class="text-sm text-muted-foreground">No messages yet.</p>
						{:else}
							<ul class="space-y-2">
								{#each messages as msg, i (i)}
									{#if msg.sender_type === 'user'}
										<li
											class="rounded-lg border-2 border-bubble-user-border bg-bubble-user px-3 py-2 text-sm text-foreground"
										>
											<div class="flex flex-col gap-1.5">
												<div class="font-semibold text-foreground">{msg.sender_name}</div>
												<div
													class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-pre:my-2 prose-strong:font-semibold prose-strong:text-foreground prose-headings:first:mt-0"
												>
													<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes -->
													{@html renderMarkdown(msg.content)}
												</div>
											</div>
										</li>
									{:else}
										{@const prefix = splitAssistantLabelPrefix(
											msg.content,
											msg.sender_model ?? 'assistant'
										)}
										<li
											class="rounded-lg border-2 border-bubble-assistant-border bg-bubble-assistant px-3 py-2 text-sm text-foreground"
										>
											{#if showReasoningControl(i, msg)}
												<div class="mb-2 flex flex-col gap-2">
													{#if !reasoningExpandedByIndex[i]}
														<div
															class="reasoning-panel overflow-hidden rounded-md border border-border/55 bg-background/40 text-left shadow-sm"
														>
															<button
																type="button"
																class="reasoning-disclosure flex w-full items-center justify-start gap-0.5 border-b border-transparent px-2 py-1.5 text-left transition-colors hover:bg-muted/30 focus-visible:outline focus-visible:ring-2 focus-visible:ring-ring"
																onclick={() => toggleReasoningExpanded(i)}
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
																class="reasoning-disclosure flex w-full items-center justify-start gap-0.5 border-b border-border/50 px-2 py-1.5 text-left transition-colors hover:bg-muted/30 focus-visible:outline focus-visible:ring-2 focus-visible:ring-ring"
																onclick={() => toggleReasoningExpanded(i)}
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
																class="max-h-64 overflow-y-auto whitespace-pre-wrap break-words px-2 py-2 font-mono text-xs leading-relaxed text-muted-foreground"
															>
																{msg.reasoning?.trim()
																	? msg.reasoning
																	: 'Waiting for reasoning…'}
															</p>
														</div>
													{/if}
												</div>
											{/if}
											<div class="flex flex-col gap-1.5">
												<div
													class="flex flex-wrap items-baseline gap-x-1.5 text-sm"
													role={showAssistantModelThinking(i, msg) ? 'status' : undefined}
													aria-live={showAssistantModelThinking(i, msg) ? 'polite' : undefined}
												>
													<span
														class="font-semibold text-foreground {showAssistantModelThinking(i, msg)
															? 'assistant-model-name-pulse'
															: ''}">{msg.sender_name}</span
													>
													<span class="inline-flex items-baseline font-normal text-muted-foreground">
														<span>({msg.sender_model ?? 'assistant'})</span>
														{#if showAssistantModelThinking(i, msg)}
															<span
																class="thinking-dots ml-1 text-muted-foreground"
																aria-hidden="true"
																><span>.</span><span>.</span><span>.</span></span
															>
														{/if}
													</span>
												</div>
												{#if prefix?.rest?.trim()}
													<div
														class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-pre:my-2 prose-strong:font-semibold prose-strong:text-foreground prose-headings:first:mt-0"
													>
														<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes LLM HTML -->
														{@html renderMarkdown(prefix.rest)}
													</div>
												{:else if !prefix && msg.content.trim()}
													<div
														class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-pre:my-2 prose-strong:font-semibold prose-strong:text-foreground prose-headings:first:mt-0"
													>
														<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes LLM HTML -->
														{@html renderMarkdown(msg.content)}
													</div>
												{/if}
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
