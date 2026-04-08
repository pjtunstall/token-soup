<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import {
		postChat,
		normalizeChatTitle,
		fallbackTitleFromUserMessage,
		type AIStreamEvent,
		type ChatMessage
	} from '$lib/chat';
	import {
		fetchRoles,
		deleteRole,
		type RoleRow
	} from '$lib/roles';
	import { fetchChats, fetchChat, type ChatSummaryRow } from '$lib/chats';
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
	let rolesPanelRef = $state<RolesPanel | null>(null);

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
		rolesPanelRef?.closeEditor();
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
		rolesPanelRef?.closeEditor();
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
						<RolesPanel
							bind:this={rolesPanelRef}
							{roleRows}
							{rolesLoading}
							{rolesLoadError}
							{selectedRoleId}
							{deletingRoleId}
							displayName={getDisplayName(session)}
							accessToken={session?.access_token ?? ''}
							onSelectRole={selectRoleAsCurrent}
							onRolesChanged={loadRoleRows}
							onDeleteRole={deleteRoleRow}
						/>
					</aside>
				</div>
			{/if}
		</CardContent>
	</Card>
</main>
