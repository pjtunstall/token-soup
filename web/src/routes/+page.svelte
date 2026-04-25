<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { postChat, normalizeChatTitle, type AIStreamEvent, type ChatMessage } from '$lib/chat';
	import type { ChatSummaryRow } from '$lib/chats';
	import type { RoleRow } from '$lib/roles';
	import {
		appendAssistantMessage,
		appendUserMessage,
		applyAnswerDelta,
		applyReasoningDelta,
		applyReplacement
	} from '$lib/chat-message-state';
	import { USER_MESSAGE_MAX_CHARS } from '$lib/limits';
	import { getDisplayName } from '$lib/session';
	import { getSupabase } from '$lib/supabase';
	import { RolesController } from '$lib/roles-controller.svelte';
	import { ChatsController } from '$lib/chats-controller.svelte';
	import type { Session } from '@supabase/supabase-js';
	import { Card, CardContent } from '$lib/components/ui/card';
	import { toggleTheme as applyToggleTheme } from '$lib/theme';
	import AuthForm from './AuthForm.svelte';
	import ChatsGallery from './ChatsGallery.svelte';
	import AppHeader from './AppHeader.svelte';
	import ChatPanel from './ChatPanel.svelte';
	import RolesPanel from './RolesPanel.svelte';

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
	let rolesPanelRef = $state<RolesPanel | null>(null);

	const LIST_BATCH_SIZE = 10;
	const HAD_SESSION_STORAGE_KEY = 'token-soup:hadSession';

	const supabase = getSupabase();

	function markSessionSeen() {
		try {
			localStorage.setItem(HAD_SESSION_STORAGE_KEY, '1');
		} catch {
			/* ignore storage failures */
		}
	}

	function hasSeenSession(): boolean {
		try {
			return localStorage.getItem(HAD_SESSION_STORAGE_KEY) === '1';
		} catch {
			return false;
		}
	}

	function rolesErrorMessage(err: unknown): string {
		const message = err instanceof Error ? err.message : String(err);
		if (message.trim().toLowerCase() === 'unauthorized' && hasSeenSession()) {
			return 'Session expired. Please sign in again.';
		}
		return message;
	}

	const roles = new RolesController({
		getAccessToken: () => session?.access_token,
		getUserId: () => session?.user?.id,
		pageSize: LIST_BATCH_SIZE,
		getStorage: () => localStorage,
		mapError: rolesErrorMessage
	});

	const chats = new ChatsController({
		getAccessToken: () => session?.access_token,
		pageSize: LIST_BATCH_SIZE
	});

	function toggleReasoningExpanded(i: number) {
		const next = !reasoningExpandedByIndex[i];
		reasoningExpandedByIndex = { ...reasoningExpandedByIndex, [i]: next };
	}

	async function requestCompletion() {
		rolesPanelRef?.closeEditor();
		if (signedInMain !== 'chat' || awaitingAi || !session?.access_token) return;
		awaitingAi = true;
		abortController = new AbortController();
		let titleReceivedThisRequest = false;
		let abortedThisRequest = false;
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
				titleReceivedThisRequest = true;
				sessionSubtitle = normalizeChatTitle(parsed.text ?? '', '');
				return;
			}
			handleStreamEvent(parsed);
		}
		try {
			await postChat(
				'',
				getDisplayName(session),
				session.access_token,
				abortController.signal,
				chatId,
				persistEphemeral,
				onStreamEvent,
				roles.selectedId,
				true
			);
		} catch (err) {
			const name = err instanceof Error ? err.name : '';
			if (name === 'AbortError') {
				abortedThisRequest = true;
			} else {
				const msg = err instanceof Error ? err.message : String(err);
				pushAssistantBubble(`Error: ${msg}`, undefined);
			}
		} finally {
			void titleReceivedThisRequest;
			void abortedThisRequest;
			awaitingAi = false;
			activeAiJobId = '';
			abortController = null;
		}
	}

	function handleDoubleClickRole(r: RoleRow) {
		roles.selectRole(r);
		void requestCompletion();
	}

	function scheduleScroll() {
		if (scrollRaf !== null) return;
		scrollRaf = requestAnimationFrame(() => {
			scrollRaf = null;
			void scrollToBottom();
		});
	}

	function pushAssistantBubble(text: string, senderModel?: string) {
		const next = appendAssistantMessage(messages, roles.assistantName, text, senderModel);
		messages = next.messages;
		scheduleScroll();
		return next.index;
	}

	function pushUserBubble(senderName: string, text: string) {
		const next = appendUserMessage(messages, senderName, text);
		messages = next.messages;
		scheduleScroll();
		return next.index;
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
				messages = applyReasoningDelta(messages, idx, roles.assistantName, deltaText, parsed.name);
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
				const answerStarted = deltaText.trim() !== '';
				if (answerStarted) {
					reasoningExpandedByIndex = { ...reasoningExpandedByIndex, [idx]: false };
				}
				messages = applyAnswerDelta(messages, idx, roles.assistantName, deltaText, parsed.name);
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
				messages = applyReplacement(messages, idx, roles.assistantName, replacement, parsed.name);
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
		roles.reset();
		rolesPanelRef?.closeEditor();
		chats.reset();
	}

	onMount(() => {
		void supabase.auth.getSession().then(({ data: { session: s } }) => {
			session = s;
			authState = s ? 'signed_in' : 'signed_out';
			if (s) markSessionSeen();
			if (s?.access_token) void roles.load();
		});

		const {
			data: { subscription }
		} = supabase.auth.onAuthStateChange((_event, s) => {
			session = s;
			authState = s ? 'signed_in' : 'signed_out';
			if (s?.access_token) {
				markSessionSeen();
				void roles.load();
			} else {
				roles.reset();
			}
		});

		return () => subscription.unsubscribe();
	});

	async function sendMessage() {
		rolesPanelRef?.closeEditor();
		if (signedInMain !== 'chat' || awaitingAi || !session?.access_token || !messageInput.trim())
			return;
		const text = messageInput.trim();
		if ([...text].length > USER_MESSAGE_MAX_CHARS) {
			pushAssistantBubble(
				`Error: Message is too long. Maximum is ${USER_MESSAGE_MAX_CHARS} characters.`,
				undefined
			);
			return;
		}
		const who = getDisplayName(session);
		pushUserBubble(who, text);
		messageInput = '';
		awaitingAi = true;
		abortController = new AbortController();
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
				sessionSubtitle = normalizeChatTitle(parsed.text ?? '', text);
				return;
			}
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
				roles.selectedId,
				false,
				true
			);
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
		rolesPanelRef?.closeEditor();
		abortController?.abort();
		awaitingAi = false;
		activeAiJobId = '';
	}

	function handleToggleTheme() {
		applyToggleTheme();
	}

	function handleSpeechBubble() {
		rolesPanelRef?.closeEditor();
		signedInMain = 'chat';
	}

	function handleChatGallery() {
		rolesPanelRef?.closeEditor();
		signedInMain = 'chats';
		void chats.load();
	}

	async function openChatFromList(c: ChatSummaryRow) {
		if (!session?.access_token) return;
		rolesPanelRef?.closeEditor();
		abortController?.abort();
		awaitingAi = false;
		activeAiJobId = '';
		abortController = null;
		aiBubbleIndexByJobId = {};
		reasoningExpandedByIndex = {};
		messageInput = '';
		persistWarning = '';
		persistEphemeral = false;
		chats.clearError();
		const detail = await chats.fetchDetail(c.id);
		if (!detail) return;
		chatId = detail.id;
		sessionSubtitle = detail.title.trim() || defaultSessionSubtitle;
		messages = detail.messages;
		signedInMain = 'chat';
		scheduleScroll();
	}

	function handleNewChat() {
		rolesPanelRef?.closeEditor();
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

	function handleDeleteRole(r: RoleRow, e: MouseEvent) {
		e.preventDefault();
		e.stopPropagation();
		void roles.deleteRow(r);
	}

	onDestroy(() => {
		abortController?.abort();
	});
</script>

<main class="mx-auto flex min-h-screen w-full max-w-3xl items-center p-4">
	<Card class="flex min-h-[80vh] w-full flex-col md:h-[80vh] md:min-h-0">
		<AppHeader
			{authState}
			{signedInMain}
			{sessionSubtitle}
			onNewChat={handleNewChat}
			onChatGallery={handleChatGallery}
			onCurrentChat={handleSpeechBubble}
			onToggleTheme={handleToggleTheme}
			onSignOut={() => void signOut()}
		>
			<svelte:fragment slot="mobile-roles-sidebar-content">
				{#if authState === 'signed_in'}
					<div class="mt-1 min-h-0 rounded-md border border-border/70 bg-field/45 p-2 md:hidden">
						<RolesPanel
							roleRows={roles.rows}
							rolesLoading={roles.loading}
							rolesLoadError={roles.loadError}
							rolesLoadingMore={roles.loadingMore}
							rolesHasMore={roles.hasMore}
							selectedRoleId={roles.selectedId}
							deletingRoleId={roles.deletingId}
							userTurn={roles.userTurn}
							displayName={getDisplayName(session)}
							accessToken={session?.access_token ?? ''}
							onSelectRole={(r) => roles.selectRole(r)}
							onSelectUser={() => roles.selectUser()}
							onDoubleClickRole={handleDoubleClickRole}
							onRolesChanged={() => roles.load()}
							onDeleteRole={handleDeleteRole}
							onLoadMore={() => void roles.loadMore()}
						/>
					</div>
				{/if}
			</svelte:fragment>
		</AppHeader>
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
				<div class="flex min-h-0 flex-1 flex-col gap-3 md:flex-row">
					<div class="flex min-h-0 min-w-0 flex-1 flex-col">
						{#if signedInMain === 'chats'}
							<div class="flex min-h-0 flex-1 flex-col">
								<ChatsGallery
									chatRows={chats.rows}
									chatsLoading={chats.loading}
									chatsLoadingMore={chats.loadingMore}
									chatsLoadError={chats.loadError}
									chatsHasMore={chats.hasMore}
									activeChatId={chatId}
									onNewChat={handleNewChat}
									onOpenChat={(c) => void openChatFromList(c)}
									onLoadMore={() => void chats.loadMore()}
								/>
							</div>
						{:else}
							<div class="flex min-h-0 flex-1 flex-col">
								<ChatPanel
									{messages}
									{awaitingAi}
									pendingAssistantName={roles.assistantName}
									bind:messageInput
									bind:logContainer
									activeStreamingIndex={activeAiJobId !== ''
										? aiBubbleIndexByJobId[activeAiJobId]
										: undefined}
									{reasoningExpandedByIndex}
									onToggleReasoning={toggleReasoningExpanded}
									onSend={sendMessage}
									onStop={stopMessage}
									aiMode={!roles.userTurn}
									onRequestCompletion={requestCompletion}
								/>
							</div>
						{/if}
					</div>
					<aside
						class="hidden w-full shrink-0 flex-col gap-2 rounded-md border border-border/70 bg-field/45 p-2 md:flex md:w-56"
						aria-label="Roles panel"
					>
						<RolesPanel
							bind:this={rolesPanelRef}
							roleRows={roles.rows}
							rolesLoading={roles.loading}
							rolesLoadError={roles.loadError}
							rolesLoadingMore={roles.loadingMore}
							rolesHasMore={roles.hasMore}
							selectedRoleId={roles.selectedId}
							deletingRoleId={roles.deletingId}
							userTurn={roles.userTurn}
							displayName={getDisplayName(session)}
							accessToken={session?.access_token ?? ''}
							onSelectRole={(r) => roles.selectRole(r)}
							onSelectUser={() => roles.selectUser()}
							onDoubleClickRole={handleDoubleClickRole}
							onRolesChanged={() => roles.load()}
							onDeleteRole={handleDeleteRole}
							onLoadMore={() => void roles.loadMore()}
						/>
					</aside>
				</div>
			{/if}
		</CardContent>
	</Card>
</main>
