<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import {
		postChat,
		normalizeChatTitle,
		type AIStreamEvent,
		type ChatMessage
	} from '$lib/chat';
	import {
		fetchRoles,
		deleteRole,
		type RoleRow
	} from '$lib/roles';
	import { fetchChats, fetchChat, type ChatSummaryRow } from '$lib/chats';
	import {
		appendAssistantMessage,
		appendUserMessage,
		applyAnswerDelta,
		applyReasoningDelta,
		applyReplacement
	} from '$lib/chat-message-state';
	import {
		activeAssistantName,
		chooseSelectedRoleId,
		getStoredSelectedRoleId,
		persistSelectedRoleId
	} from '$lib/role-selection';
	import { loadFirstPage, loadNextPage } from '$lib/pagination';
	import { getDisplayName } from '$lib/session';
	import { getSupabase } from '$lib/supabase';
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
	let roleRows = $state<RoleRow[]>([]);
	let rolesLoadError = $state('');
	let rolesLoading = $state(false);
	let deletingRoleId = $state<number | null>(null);
	/** Persisted AI persona (`public.roles`); drives system prompt and assistant label. */
	let selectedRoleId = $state<number | null>(null);
	/** When true the user is typing; when false the selected AI role will generate next. */
	let userTurn = $state(false);
	let rolesPanelRef = $state<RolesPanel | null>(null);

	let chatRows = $state<ChatSummaryRow[]>([]);
	let chatsLoadError = $state('');
	let chatsLoading = $state(false);
	let chatsLoadingMore = $state(false);
	let chatsHasMore = $state(true);

	const LIST_BATCH_SIZE = 10;

	const supabase = getSupabase();

	function toggleReasoningExpanded(i: number) {
		const next = !reasoningExpandedByIndex[i];
		reasoningExpandedByIndex = { ...reasoningExpandedByIndex, [i]: next };
	}

	function currentAssistantName(): string {
		return activeAssistantName(roleRows, selectedRoleId);
	}

	function syncSelectedRoleAfterLoad() {
		const u = session?.user;
		if (!u?.id) return;
		const rawStoredId = getStoredSelectedRoleId(localStorage, u.id);
		const picked = chooseSelectedRoleId(roleRows, rawStoredId);
		selectedRoleId = picked;
		if (picked != null) {
			persistSelectedRoleId(localStorage, u.id, picked);
		}
	}

	function selectRoleAsCurrent(r: RoleRow) {
		selectedRoleId = r.id;
		userTurn = false;
		const uid = session?.user?.id;
		if (uid) {
			persistSelectedRoleId(localStorage, uid, r.id);
		}
	}

	function selectUserTurn() {
		userTurn = true;
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
				selectedRoleId,
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
		selectRoleAsCurrent(r);
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
		const next = appendAssistantMessage(messages, currentAssistantName(), text, senderModel);
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
				messages = applyReasoningDelta(
					messages,
					idx,
					currentAssistantName(),
					deltaText,
					parsed.name
				);
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
				messages = applyAnswerDelta(messages, idx, currentAssistantName(), deltaText, parsed.name);
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
				messages = applyReplacement(
					messages,
					idx,
					currentAssistantName(),
					replacement,
					parsed.name
				);
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
		rolesHasMore = true;
		rolesLoadingMore = false;
		selectedRoleId = null;
		rolesPanelRef?.closeEditor();
		chatRows = [];
		chatsLoadError = '';
		chatsHasMore = true;
		chatsLoadingMore = false;
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
				rolesHasMore = true;
				rolesLoadingMore = false;
			}
		});

		return () => subscription.unsubscribe();
	});

	async function sendMessage() {
		rolesPanelRef?.closeEditor();
		if (signedInMain !== 'chat' || awaitingAi || !session?.access_token || !messageInput.trim())
			return;
		const text = messageInput.trim();
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
				selectedRoleId,
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

	async function loadChatRows() {
		const accessToken = session?.access_token;
		if (!accessToken) return;
		chatsLoading = true;
		chatsLoadError = '';
		try {
			const page = await loadFirstPage((params) => fetchChats(accessToken, params), LIST_BATCH_SIZE);
			chatRows = page.rows;
			chatsHasMore = page.hasMore;
		} catch (e) {
			chatRows = [];
			chatsHasMore = true;
			chatsLoadError = e instanceof Error ? e.message : String(e);
		} finally {
			chatsLoading = false;
		}
	}

	async function loadMoreChats() {
		const accessToken = session?.access_token;
		if (!accessToken || chatsLoading || chatsLoadingMore || !chatsHasMore) return;
		chatsLoadingMore = true;
		try {
			const page = await loadNextPage((params) => fetchChats(accessToken, params), LIST_BATCH_SIZE, chatRows);
			chatRows = page.rows;
			chatsHasMore = page.hasMore;
		} catch (e) {
			chatsLoadError = e instanceof Error ? e.message : String(e);
		} finally {
			chatsLoadingMore = false;
		}
	}

	function handleChatGallery() {
		rolesPanelRef?.closeEditor();
		signedInMain = 'chats';
		void loadChatRows();
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

	async function loadRoleRows() {
		const accessToken = session?.access_token;
		if (!accessToken) return;
		rolesLoading = true;
		rolesLoadError = '';
		try {
			const page = await loadFirstPage((params) => fetchRoles(accessToken, params), LIST_BATCH_SIZE);
			roleRows = page.rows;
			rolesHasMore = page.hasMore;
		} catch (e) {
			roleRows = [];
			rolesHasMore = true;
			rolesLoadError = e instanceof Error ? e.message : String(e);
		} finally {
			rolesLoading = false;
		}
		syncSelectedRoleAfterLoad();
	}

	let rolesLoadingMore = $state(false);
	let rolesHasMore = $state(true);

	async function loadMoreRoles() {
		const accessToken = session?.access_token;
		if (!accessToken || rolesLoading || rolesLoadingMore || !rolesHasMore) return;
		rolesLoadingMore = true;
		try {
			const page = await loadNextPage((params) => fetchRoles(accessToken, params), LIST_BATCH_SIZE, roleRows);
			roleRows = page.rows;
			rolesHasMore = page.hasMore;
		} catch (e) {
			rolesLoadError = e instanceof Error ? e.message : String(e);
		} finally {
			rolesLoadingMore = false;
		}
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
									{chatsLoadingMore}
									{chatsLoadError}
									{chatsHasMore}
									activeChatId={chatId}
									onNewChat={handleNewChat}
									onOpenChat={(c) => void openChatFromList(c)}
									onLoadMore={() => void loadMoreChats()}
								/>
							</div>
						{:else}
							<div class="flex min-h-0 flex-1 flex-col">
								<ChatPanel
									{messages}
									{awaitingAi}
									pendingAssistantName={currentAssistantName()}
									bind:messageInput
									bind:logContainer
									activeStreamingIndex={activeAiJobId !== ''
										? aiBubbleIndexByJobId[activeAiJobId]
										: undefined}
									{reasoningExpandedByIndex}
									onToggleReasoning={toggleReasoningExpanded}
									onSend={sendMessage}
									onStop={stopMessage}
									aiMode={!userTurn}
									onRequestCompletion={requestCompletion}
								/>
							</div>
						{/if}
					</div>
					<aside
						class="flex w-56 shrink-0 flex-col gap-2 rounded-md border border-border/70 bg-field/45 p-2"
						aria-label="Roles panel"
					>
						<RolesPanel
							bind:this={rolesPanelRef}
							{roleRows}
							{rolesLoading}
							{rolesLoadError}
							{rolesLoadingMore}
							{rolesHasMore}
							{selectedRoleId}
							{deletingRoleId}
							{userTurn}
							displayName={getDisplayName(session)}
							accessToken={session?.access_token ?? ''}
							onSelectRole={selectRoleAsCurrent}
							onSelectUser={selectUserTurn}
							onDoubleClickRole={handleDoubleClickRole}
							onRolesChanged={loadRoleRows}
							onDeleteRole={deleteRoleRow}
							onLoadMore={() => void loadMoreRoles()}
						/>
					</aside>
				</div>
			{/if}
		</CardContent>
	</Card>
</main>
