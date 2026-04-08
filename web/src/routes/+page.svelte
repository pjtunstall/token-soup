<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import {
		postChat,
		normalizeChatTitle,
		fallbackTitleFromUserMessage,
		type AIStreamEvent,
		type ChatMessage
	} from '$lib/chat';
	import { fetchRoles, createRole, updateRoleDescription, deleteRole, type RoleRow } from '$lib/roles';
	import { fetchChats, fetchChat, type ChatSummaryRow } from '$lib/chats';
	import { getSupabase } from '$lib/supabase';
	import type { Session } from '@supabase/supabase-js';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { toggleTheme as applyToggleTheme } from '$lib/theme';
	import Feather from '@lucide/svelte/icons/feather';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import AuthForm from './AuthForm.svelte';
	import ChatsGallery from './ChatsGallery.svelte';
	import AppHeader from './AppHeader.svelte';
	import ChatPanel from './ChatPanel.svelte';

	let authState = $state<'loading' | 'signed_out' | 'signed_in'>('loading');
	let session = $state<Session | null>(null);

	let messageInput = $state('');
	let messages = $state<ChatMessage[]>([]);
	let awaitingAi = $state(false);
	let activeAiJobId = $state<string>('');
	let logContainer = $state<HTMLDivElement | null>(null);
	let aiBubbleIndexByJobId = $state<Record<string, number>>({});
	/** Per-message index: expanded reasoning panel (click-to-reveal). */
	let reasoningExpandedByIndex = $state<Record<number, boolean>>({});
	let scrollRaf: number | null = null;
	let abortController: AbortController | null = null;

	const defaultSessionSubtitle = 'What do I get into today?';
	let sessionSubtitle = $state(defaultSessionSubtitle);
	/** Set after the server creates a row in `public.chats` (first persisted message). */
	let chatId = $state<number | null>(null);
	/** Non-empty while the user should know persistence failed (cleared on `chat_session` or sign-out). */
	let persistWarning = $state('');
	/** After persist_warning on a new chat, keep turns in-memory only (tells the API not to create a new DB chat each send). */
	let persistEphemeral = $state(false);
	/** Signed-in main panel: chat or gallery (`chats`). */
	let signedInMain = $state<'chat' | 'chats'>('chat');
	let roleRows = $state<RoleRow[]>([]);
	let rolesLoadError = $state('');
	let rolesLoading = $state(false);
	let deletingRoleId = $state<number | null>(null);
	/** Persisted AI persona (`public.roles`); drives system prompt and assistant label. */
	let selectedRoleId = $state<number | null>(null);
	let roleEditorOpen = $state(false);
	let roleEditorIsNew = $state(false);
	let roleEditorRoleId = $state<number | null>(null);
	let roleEditorName = $state('');
	let roleEditorDescription = $state('');
	let roleEditorSaving = $state(false);
	let roleEditorError = $state('');

	let chatRows = $state<ChatSummaryRow[]>([]);
	let chatsLoadError = $state('');
	let chatsLoading = $state(false);

	const supabase = getSupabase();

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

	function selectedRoleStorageKey(uid: string): string {
		return `token-soup:selectedRoleId:${uid}`;
	}

	function activeAssistantName(): string {
		if (selectedRoleId != null) {
			const row = roleRows.find((r) => r.id === selectedRoleId);
			if (row) return row.name;
		}
		return 'Guru';
	}

	function syncSelectedRoleAfterLoad() {
		const u = session?.user;
		if (!u?.id) return;
		let picked: number | null = null;
		try {
			const raw = localStorage.getItem(selectedRoleStorageKey(u.id));
			if (raw) {
				const id = Number(raw);
				if (Number.isFinite(id) && roleRows.some((r) => r.id === id)) picked = id;
			}
		} catch {
			/* ignore */
		}
		if (picked == null) {
			const guru = roleRows.find((r) => r.name.toLowerCase() === 'guru');
			if (guru) picked = guru.id;
			else if (roleRows.length > 0) picked = roleRows[0]!.id;
		}
		selectedRoleId = picked;
		if (picked != null) {
			try {
				localStorage.setItem(selectedRoleStorageKey(u.id), String(picked));
			} catch {
				/* ignore */
			}
		}
	}

	function selectRoleAsCurrent(r: RoleRow) {
		selectedRoleId = r.id;
		const uid = session?.user?.id;
		if (uid) {
			try {
				localStorage.setItem(selectedRoleStorageKey(uid), String(r.id));
			} catch {
				/* ignore */
			}
		}
	}

	function closeRoleEditor() {
		roleEditorOpen = false;
		roleEditorIsNew = false;
		roleEditorRoleId = null;
		roleEditorName = '';
		roleEditorDescription = '';
		roleEditorError = '';
		roleEditorSaving = false;
	}

	function openNewRoleEditor() {
		roleEditorOpen = true;
		roleEditorIsNew = true;
		roleEditorRoleId = null;
		roleEditorName = '';
		roleEditorDescription = '';
		roleEditorError = '';
	}

	function openExistingRoleEditor(r: RoleRow) {
		roleEditorOpen = true;
		roleEditorIsNew = false;
		roleEditorRoleId = r.id;
		roleEditorName = r.name;
		roleEditorDescription = r.description ?? '';
		roleEditorError = '';
	}

	async function saveRoleEditor() {
		if (!session?.access_token || !roleEditorOpen || roleEditorSaving) return;
		roleEditorError = '';
		roleEditorSaving = true;
		try {
			if (roleEditorIsNew) {
				const name = roleEditorName.trim();
				if (!name) {
					roleEditorError = 'Role name is required.';
					return;
				}
				await createRole(session.access_token, name, roleEditorDescription.trim() || null);
			} else if (roleEditorRoleId != null) {
				await updateRoleDescription(
					session.access_token,
					roleEditorRoleId,
					roleEditorDescription.trim() || null
				);
			}
			await loadRoleRows();
			closeRoleEditor();
		} catch (e) {
			roleEditorError = e instanceof Error ? e.message : String(e);
		} finally {
			roleEditorSaving = false;
		}
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
				sender_name: activeAssistantName(),
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
						sender_name: activeAssistantName(),
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
						sender_name: activeAssistantName(),
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
				const nm = cur?.sender_type === 'bot' ? (cur.sender_model ?? parsed.name) : parsed.name;
				const keepReasoning = cur?.sender_type === 'bot' ? cur.reasoning : undefined;
				messages = [
					...messages.slice(0, idx),
					{
						sender_type: 'bot',
						sender_name: activeAssistantName(),
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

	async function signOut() {
		await supabase.auth.signOut();
		messages = [];
		aiBubbleIndexByJobId = {};
		reasoningExpandedByIndex = {};
		sessionSubtitle = defaultSessionSubtitle;
		chatId = null;
		persistWarning = '';
		persistEphemeral = false;
		signedInMain = 'chat';
		roleRows = [];
		rolesLoadError = '';
		selectedRoleId = null;
		closeRoleEditor();
		chatRows = [];
		chatsLoadError = '';
	}

	onMount(() => {
		void supabase.auth.getSession().then(({ data: { session: s } }) => {
			session = s;
			authState = s ? 'signed_in' : 'signed_out';
			if (s?.access_token) void loadRoleRows();
		});

		const {
			data: { subscription }
		} = supabase.auth.onAuthStateChange((_event, s) => {
			session = s;
			authState = s ? 'signed_in' : 'signed_out';
			if (s?.access_token) {
				void loadRoleRows();
			} else {
				selectedRoleId = null;
				roleRows = [];
			}
		});

		return () => subscription.unsubscribe();
	});

	async function sendMessage() {
		closeRoleEditor();
		if (signedInMain !== 'chat' || awaitingAi || !session?.access_token || !messageInput.trim())
			return;
		const wasEmpty = messages.length === 0;
		const text = messageInput.trim();
		const who = getDisplayName(session);
		pushUserBubble(who, text);
		messageInput = '';
		awaitingAi = true;
		abortController = new AbortController();
		let titleReceivedThisSend = false;
		let abortedThisSend = false;
		function onStreamEvent(parsed: AIStreamEvent) {
			if (parsed.type === 'chat_session' && typeof parsed.chatId === 'number') {
				chatId = parsed.chatId;
				persistWarning = '';
				persistEphemeral = false;
				return;
			}
			if (parsed.type === 'persist_warning') {
				persistWarning =
					(parsed.text ?? '').trim() || 'Your chat could not be saved to the server.';
				if (chatId == null) {
					persistEphemeral = true;
				}
				return;
			}
			if (parsed.type === 'chat_title') {
				titleReceivedThisSend = true;
				sessionSubtitle = normalizeChatTitle(parsed.text ?? '', text);
				return;
			}
			handleStreamEvent(parsed);
		}
		try {
			await postChat(
				text,
				who,
				session.access_token,
				abortController.signal,
				chatId,
				persistEphemeral,
				onStreamEvent,
				selectedRoleId
			);
		} catch (err) {
			const name = err instanceof Error ? err.name : '';
			if (name === 'AbortError') {
				abortedThisSend = true;
			} else {
				const msg = err instanceof Error ? err.message : String(err);
				pushAssistantBubble(`Error: ${msg}`, undefined);
			}
		} finally {
			// Only the stop button should use the local first-message heuristic; a normal
			// completion gets chat_title from the server (including server-side fallback if the title LLM fails).
			if (wasEmpty && abortedThisSend && !titleReceivedThisSend) {
				sessionSubtitle = fallbackTitleFromUserMessage(text);
			}
			awaitingAi = false;
			activeAiJobId = '';
			abortController = null;
		}
	}

	function stopMessage() {
		closeRoleEditor();
		abortController?.abort();
		awaitingAi = false;
		activeAiJobId = '';
	}

	function handleToggleTheme() {
		applyToggleTheme();
	}

	function handleSpeechBubble() {
		closeRoleEditor();
		signedInMain = 'chat';
	}

	async function loadChatRows() {
		if (!session?.access_token) return;
		chatsLoading = true;
		chatsLoadError = '';
		try {
			chatRows = await fetchChats(session.access_token);
		} catch (e) {
			chatRows = [];
			chatsLoadError = e instanceof Error ? e.message : String(e);
		} finally {
			chatsLoading = false;
		}
	}

	function handleChatGallery() {
		closeRoleEditor();
		signedInMain = 'chats';
		void loadChatRows();
	}

	async function openChatFromList(c: ChatSummaryRow) {
		if (!session?.access_token) return;
		closeRoleEditor();
		abortController?.abort();
		awaitingAi = false;
		activeAiJobId = '';
		abortController = null;
		aiBubbleIndexByJobId = {};
		reasoningExpandedByIndex = {};
		messageInput = '';
		persistWarning = '';
		persistEphemeral = false;
		chatsLoadError = '';
		try {
			const detail = await fetchChat(session.access_token, c.id);
			chatId = detail.id;
			sessionSubtitle = detail.title.trim() || defaultSessionSubtitle;
			messages = detail.messages;
			signedInMain = 'chat';
			scheduleScroll();
		} catch (e) {
			chatsLoadError = e instanceof Error ? e.message : String(e);
		}
	}

	function handleNewChat() {
		closeRoleEditor();
		abortController?.abort();
		awaitingAi = false;
		activeAiJobId = '';
		abortController = null;
		messages = [];
		aiBubbleIndexByJobId = {};
		reasoningExpandedByIndex = {};
		messageInput = '';
		sessionSubtitle = defaultSessionSubtitle;
		chatId = null;
		persistWarning = '';
		persistEphemeral = false;
		signedInMain = 'chat';
	}

	async function loadRoleRows() {
		if (!session?.access_token) return;
		rolesLoading = true;
		rolesLoadError = '';
		try {
			roleRows = await fetchRoles(session.access_token);
		} catch (e) {
			roleRows = [];
			rolesLoadError = e instanceof Error ? e.message : String(e);
		} finally {
			rolesLoading = false;
		}
		syncSelectedRoleAfterLoad();
	}

	async function deleteRoleRow(r: RoleRow, e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		if (!session?.access_token || deletingRoleId != null) return;
		deletingRoleId = r.id;
		rolesLoadError = '';
		try {
			await deleteRole(session.access_token, r.id);
			await loadRoleRows();
		} catch (err) {
			rolesLoadError = err instanceof Error ? err.message : String(err);
		} finally {
			deletingRoleId = null;
		}
	}

	onDestroy(() => {
		abortController?.abort();
	});
</script>

<main class="mx-auto flex min-h-screen w-full max-w-3xl items-center p-4">
	<Card class="flex h-[80vh] min-h-0 w-full flex-col">
		<AppHeader
			{authState}
			{signedInMain}
			{sessionSubtitle}
			onNewChat={handleNewChat}
			onChatGallery={handleChatGallery}
			onCurrentChat={handleSpeechBubble}
			onToggleTheme={handleToggleTheme}
			onSignOut={() => void signOut()}
		/>
		<CardContent class="flex min-h-0 flex-1 flex-col gap-3">
			{#if persistWarning}
				<div
					class="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-foreground"
					role="status"
				>
					{persistWarning}
				</div>
			{/if}
			{#if authState === 'loading'}
				<p class="text-sm text-muted-foreground">Loading…</p>
			{:else if authState === 'signed_out'}
				<AuthForm {supabase} />
			{:else}
				<div class="flex min-h-0 flex-1 gap-3">
					<div class="flex min-h-0 min-w-0 flex-1 flex-col">
						{#if signedInMain === 'chats'}
							<div class="flex min-h-0 flex-1 flex-col">
								<ChatsGallery
									{chatRows}
									{chatsLoading}
									{chatsLoadError}
									activeChatId={chatId}
									onNewChat={handleNewChat}
									onOpenChat={(c) => void openChatFromList(c)}
								/>
							</div>
						{:else}
							<div class="flex min-h-0 flex-1 flex-col">
								<ChatPanel
									{messages}
									{awaitingAi}
									bind:messageInput
									bind:logContainer
									activeStreamingIndex={activeAiJobId !== ''
										? aiBubbleIndexByJobId[activeAiJobId]
										: undefined}
									{reasoningExpandedByIndex}
									onToggleReasoning={toggleReasoningExpanded}
									onSend={sendMessage}
									onStop={stopMessage}
								/>
							</div>
						{/if}
					</div>
					<aside
						class="flex w-56 shrink-0 flex-col gap-2 rounded-md border border-border/70 bg-field/45 p-2"
						aria-label="Roles panel"
					>
						<Button
							type="button"
							size="sm"
							variant="outline"
							class="header-control-btn h-8 w-full justify-start truncate px-2 text-sm disabled:opacity-100"
							disabled
							aria-label={roleEditorOpen ? 'Editing role' : 'Signed in user'}
							onclick={() => {
								closeRoleEditor();
							}}
						>
							{roleEditorOpen ? roleEditorName || 'New role' : getDisplayName(session)}
						</Button>
						<div class="min-h-0 flex-1">
							{#if roleEditorOpen}
								<div class="flex h-full min-h-0 flex-col gap-2">
									{#if roleEditorIsNew}
										<div class="shrink-0 space-y-2">
											<Input
												type="text"
												bind:value={roleEditorName}
												placeholder="Role name"
												disabled={roleEditorSaving}
												autocomplete="off"
												class="h-8 text-sm"
											/>
										</div>
									{/if}
									<div class="min-h-0 flex-1">
										<textarea
											class="h-full min-h-[10rem] w-full resize-none rounded-md border border-input bg-field px-2 py-1.5 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
											placeholder={roleEditorIsNew
												? 'Describe how this role should behave...'
												: 'Edit role description...'}
											bind:value={roleEditorDescription}
											disabled={roleEditorSaving}
										></textarea>
										{#if roleEditorError}
											<p class="mt-2 text-xs text-destructive">{roleEditorError}</p>
										{/if}
									</div>
									<div class="shrink-0">
										<div class="flex gap-2">
											<Button
												type="button"
												variant="outline"
												size="sm"
												class="header-control-btn flex-1"
												onclick={closeRoleEditor}
											>
												Exit
											</Button>
											<Button
												type="button"
												size="sm"
												class="flex-1"
												disabled={roleEditorSaving || (roleEditorIsNew && !roleEditorName.trim())}
												onclick={() => void saveRoleEditor()}
											>
												Save
											</Button>
										</div>
									</div>
								</div>
							{:else}
								<div class="min-h-0 h-full space-y-2 overflow-y-auto">
								{#if rolesLoading}
									<p class="text-xs text-muted-foreground">Loading roles…</p>
								{/if}
								{#if rolesLoadError}
									<p class="text-xs text-destructive">{rolesLoadError}</p>
								{/if}
								<Button
									type="button"
									variant="outline"
									size="sm"
									class="header-control-btn w-full justify-start"
									onclick={openNewRoleEditor}
								>
									New role
								</Button>
								<ul class="space-y-1.5">
									{#each roleRows as r (r.id)}
										<li class="flex items-center gap-1">
											<button
												type="button"
												class="menu-action-btn role-option-btn min-w-0 flex-1 truncate rounded-md border px-2 py-1.5 text-left text-sm transition-colors {selectedRoleId ===
												r.id
													? 'border-primary/55'
													: 'border-border bg-background/40 dark:border-input dark:bg-input/30'}"
												onclick={() => {
													selectRoleAsCurrent(r);
												}}
												aria-pressed={selectedRoleId === r.id}
												data-selected={selectedRoleId === r.id ? 'true' : undefined}
											>
												{r.name}
											</button>
											<Button
												type="button"
												variant="outline"
												size="icon-sm"
												class="menu-action-btn shrink-0"
												onclick={() => openExistingRoleEditor(r)}
												aria-label={`Edit role ${r.name}`}
											>
												<Feather class="size-4" aria-hidden="true" />
											</Button>
											<Button
												type="button"
												variant="outline"
												size="icon-sm"
												class="menu-action-btn shrink-0"
												disabled={deletingRoleId === r.id}
												onclick={(e: MouseEvent) => void deleteRoleRow(r, e)}
												aria-label={`Delete role ${r.name}`}
											>
												<Trash2 class="size-4" aria-hidden="true" />
											</Button>
										</li>
									{/each}
								</ul>
								</div>
							{/if}
						</div>
					</aside>
				</div>
			{/if}
		</CardContent>
	</Card>

</main>

<style>
	:global(html:not(.dark) .menu-action-btn:hover) {
		background-color: color-mix(in oklch, var(--primary) 22%, var(--background)) !important;
		border-color: color-mix(in oklch, var(--primary) 56%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 82%, var(--primary));
	}

	:global(html:not(.dark) .role-option-btn[data-selected='true']) {
		background-color: color-mix(in oklch, var(--primary) 22%, var(--background)) !important;
		border-color: color-mix(in oklch, var(--primary) 62%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 88%, var(--primary));
	}

	:global(html.dark .role-option-btn:hover) {
		background-color: color-mix(in oklch, var(--card) 72%, black 28%) !important;
		border-color: color-mix(in oklch, var(--primary) 58%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 90%, var(--primary));
	}

	:global(html.dark .role-option-btn[data-selected='true']) {
		background-color: color-mix(in oklch, var(--card) 72%, black 28%) !important;
		border-color: color-mix(in oklch, var(--primary) 60%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 94%, var(--primary));
		box-shadow: inset 0 0 0 1px color-mix(in oklch, var(--primary) 34%, transparent);
	}
</style>
