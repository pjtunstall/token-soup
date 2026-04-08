<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { fade } from 'svelte/transition';
	import { renderMarkdown, splitAssistantLabelPrefix } from '$lib/markdown';
	import {
		postChat,
		normalizeChatTitle,
		fallbackTitleFromUserMessage,
		type AIStreamEvent,
		type ChatMessage
	} from '$lib/chat';
	import {
		fetchRoles,
		createRole,
		updateRoleDescription,
		deleteRole,
		type RoleRow
	} from '$lib/roles';
	import { fetchChats, fetchChat, type ChatSummaryRow } from '$lib/chats';
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
	import Drama from '@lucide/svelte/icons/drama';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import Feather from '@lucide/svelte/icons/feather';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import GalleryVerticalEnd from '@lucide/svelte/icons/gallery-vertical-end';
	import MessagesSquare from '@lucide/svelte/icons/messages-square';
	import Plus from '@lucide/svelte/icons/plus';
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
	/** Set after the server creates a row in `public.chats` (first persisted message). */
	let chatId = $state<number | null>(null);
	/** Non-empty while the user should know persistence failed (cleared on `chat_session` or sign-out). */
	let persistWarning = $state('');
	/** After persist_warning on a new chat, keep turns in-memory only (tells the API not to create a new DB chat each send). */
	let persistEphemeral = $state(false);
	/** Signed-in main panel: chat → idle (chat removed) → roles; `chats` = gallery / all-chats view. */
	let signedInMain = $state<'chat' | 'chats' | 'idle' | 'roles'>('chat');
	/** Bumped when cancelling `idle` so a pending `tick()` cannot still open roles. */
	let rolesEnterGeneration = 0;
	let roleRows = $state<RoleRow[]>([]);
	let rolesLoadError = $state('');
	let rolesLoading = $state(false);
	let rolesUI = $state<'list' | 'detail'>('list');
	let detailIsNew = $state(false);
	let detailRoleId = $state<number | null>(null);
	let detailName = $state('');
	let detailDescription = $state('');
	let savingRole = $state(false);
	let roleSaveError = $state('');
	let deletingRoleId = $state<number | null>(null);
	/** Persisted AI persona (`public.roles`); drives system prompt and assistant label. */
	let selectedRoleId = $state<number | null>(null);
	/** Header identity chip selection (mutually exclusive). */
	let selectedHeaderIdentity = $state<'user' | 'role'>('role');

	let chatRows = $state<ChatSummaryRow[]>([]);
	let chatsLoadError = $state('');
	let chatsLoading = $state(false);

	const supabase = getSupabase();

	function formatChatListDate(iso: string): string {
		const d = new Date(iso);
		if (Number.isNaN(d.getTime())) return '';
		return d.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
	}

	function assistantStreamContext(i: number, msg: ChatMessage) {
		if (msg.sender_type !== 'bot') {
			return { streamingHere: false, hasReasoning: false };
		}
		const activeIdx = activeAiJobId !== '' ? aiBubbleIndexByJobId[activeAiJobId] : undefined;
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
			signUpMessage = 'Check your email for a confirmation link. After you confirm, sign in here.';
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
		chatId = null;
		persistWarning = '';
		persistEphemeral = false;
		signedInMain = 'chat';
		rolesUI = 'list';
		roleRows = [];
		rolesLoadError = '';
		selectedRoleId = null;
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
		abortController?.abort();
		awaitingAi = false;
		activeAiJobId = '';
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (signedInMain !== 'chat') return;
		void sendMessage();
	}

	function handleToggleTheme() {
		applyToggleTheme();
	}

	function handleSpeechBubble() {
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
		signedInMain = 'chats';
		void loadChatRows();
	}

	async function openChatFromList(c: ChatSummaryRow) {
		if (!session?.access_token) return;
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

	function openRoleList() {
		rolesUI = 'list';
		detailIsNew = false;
		detailRoleId = null;
		detailName = '';
		detailDescription = '';
		roleSaveError = '';
	}

	function openNewRoleEditor() {
		rolesUI = 'detail';
		detailIsNew = true;
		detailRoleId = null;
		detailName = '';
		detailDescription = '';
		roleSaveError = '';
	}

	function openExistingRoleEditor(r: RoleRow) {
		rolesUI = 'detail';
		detailIsNew = false;
		detailRoleId = r.id;
		detailName = r.name;
		detailDescription = r.description ?? '';
		roleSaveError = '';
	}

	async function saveRoleFromEditor() {
		if (!session?.access_token || savingRole || rolesUI !== 'detail') return;
		roleSaveError = '';
		savingRole = true;
		try {
			if (detailIsNew) {
				const name = detailName.trim();
				if (!name) {
					savingRole = false;
					return;
				}
				await createRole(session.access_token, name, detailDescription.trim() || null);
			} else if (detailRoleId != null) {
				await updateRoleDescription(
					session.access_token,
					detailRoleId,
					detailDescription.trim() || null
				);
			}
			await loadRoleRows();
			openRoleList();
		} catch (e) {
			roleSaveError = e instanceof Error ? e.message : String(e);
		} finally {
			savingRole = false;
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
			if (detailRoleId === r.id) {
				openRoleList();
			}
			await loadRoleRows();
		} catch (err) {
			rolesLoadError = err instanceof Error ? err.message : String(err);
		} finally {
			deletingRoleId = null;
		}
	}

	function roleSendDisabled(): boolean {
		if (signedInMain !== 'roles') return true;
		if (rolesUI === 'list') return true;
		if (savingRole) return true;
		if (detailIsNew) return !detailName.trim();
		return false;
	}

	function handleRoleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (roleSendDisabled()) return;
		void saveRoleFromEditor();
	}

	async function toggleEditRolesMode() {
		if (signedInMain === 'roles') {
			return;
		}
		if (signedInMain === 'idle') {
			signedInMain = 'chat';
			rolesEnterGeneration++;
			return;
		}
		const gen = ++rolesEnterGeneration;
		signedInMain = 'idle';
		await tick();
		if (gen !== rolesEnterGeneration || signedInMain !== 'idle') return;
		signedInMain = 'roles';
		roleSaveError = '';
		openRoleList();
		void loadRoleRows();
	}

	function handleNewCharacter() {
		void toggleEditRolesMode();
	}

	/** Header icon hover labels (fixed; panel uses bg-card + shadow-card to read off the header). */
	let headerHoverTip = $state<{ text: string; x: number; y: number } | null>(null);
	let headerLeftEl = $state<HTMLElement | null>(null);
	let headerControlsEl = $state<HTMLElement | null>(null);
	let headerLeftInset = $state(0);
	let headerRightInset = $state(0);

	function updateHeaderCenterInsets() {
		headerLeftInset = headerLeftEl?.offsetWidth ?? 0;
		headerRightInset = headerControlsEl?.offsetWidth ?? 0;
	}

	function showHeaderHoverTip(text: string, e: PointerEvent) {
		const el = e.currentTarget;
		if (!(el instanceof HTMLElement)) return;
		const r = el.getBoundingClientRect();
		headerHoverTip = { text, x: r.left + r.width / 2, y: r.bottom + 6 };
	}

	function hideHeaderHoverTip() {
		headerHoverTip = null;
	}

	onMount(() => {
		const ro = new ResizeObserver(() => updateHeaderCenterInsets());
		if (headerLeftEl) ro.observe(headerLeftEl);
		if (headerControlsEl) ro.observe(headerControlsEl);
		updateHeaderCenterInsets();
		window.addEventListener('resize', updateHeaderCenterInsets);
		return () => {
			ro.disconnect();
			window.removeEventListener('resize', updateHeaderCenterInsets);
		};
	});

	onDestroy(() => {
		abortController?.abort();
	});
</script>

<main class="mx-auto flex min-h-screen w-full max-w-3xl items-center p-4">
	<Card class="flex h-[80vh] min-h-0 w-full flex-col">
		<CardHeader class="relative flex shrink-0 flex-row items-center gap-2 space-y-0">
			<div bind:this={headerLeftEl} class="w-fit max-w-[20rem] min-w-0 shrink space-y-1">
				<CardTitle class="text-primary">Token Soup</CardTitle>
				<CardDescription class="chat-subtitle-clamp break-words"
					>{signedInMain === 'roles'
						? 'Roles'
						: signedInMain === 'chats'
							? 'Chats'
							: sessionSubtitle}</CardDescription
				>
			</div>
			<div class="min-w-0 flex-1" aria-hidden="true"></div>
			<div bind:this={headerControlsEl} class="flex shrink-0 items-center gap-2">
				<Button
					type="button"
					variant="outline"
					size="icon-sm"
					class="header-control-btn shrink-0"
					onpointerenter={(e) => showHeaderHoverTip('New chat', e)}
					onpointerleave={hideHeaderHoverTip}
					onclick={handleNewChat}
					aria-label="New chat"
				>
					<Plus class="size-4" />
				</Button>
				<Button
					type="button"
					variant={signedInMain === 'chats' ? 'secondary' : 'outline'}
					size="icon-sm"
					class="header-control-btn shrink-0"
					onpointerenter={(e) => showHeaderHoverTip('Chats', e)}
					onpointerleave={hideHeaderHoverTip}
					onclick={handleChatGallery}
					aria-label="Chats"
					aria-pressed={signedInMain === 'chats'}
				>
					<GalleryVerticalEnd class="size-4" />
				</Button>
				<Button
					type="button"
					variant={signedInMain === 'chat' ? 'secondary' : 'outline'}
					size="icon-sm"
					class="header-control-btn shrink-0"
					onpointerenter={(e) => showHeaderHoverTip('Current chat', e)}
					onpointerleave={hideHeaderHoverTip}
					onclick={handleSpeechBubble}
					aria-label="Current chat"
					aria-pressed={signedInMain === 'chat'}
				>
					<MessagesSquare class="size-4" />
				</Button>
				<Button
					type="button"
					variant={signedInMain === 'roles' ? 'secondary' : 'outline'}
					size="icon-sm"
					class="header-control-btn shrink-0"
					onpointerenter={(e) => showHeaderHoverTip('Roles', e)}
					onpointerleave={hideHeaderHoverTip}
					onclick={handleNewCharacter}
					aria-label="Roles"
					aria-pressed={signedInMain === 'roles'}
				>
					<Drama class="size-4" />
				</Button>
				<Button
					type="button"
					variant="outline"
					size="icon-sm"
					class="header-control-btn shrink-0"
					onpointerenter={(e) => showHeaderHoverTip('Theme', e)}
					onpointerleave={hideHeaderHoverTip}
					onclick={handleToggleTheme}
					aria-label="Toggle color theme"
				>
					<Sun class="hidden size-4 dark:block" />
					<Moon class="size-4 dark:hidden" />
				</Button>
				{#if authState === 'signed_in'}
					<Button type="button" variant="outline" size="sm" onclick={() => void signOut()}
						>Sign out</Button
					>
				{:else if authState === 'signed_out'}
					<Badge variant="secondary">signed out</Badge>
				{:else}
					<Badge variant="secondary">…</Badge>
				{/if}
			</div>
			{#if authState === 'signed_in'}
				<div
					class="absolute inset-y-0 flex items-center"
					style:left="{headerLeftInset + 8}px"
					style:right="{headerRightInset + 8}px"
				>
					<div
						class="mx-auto flex w-fit max-w-full min-w-0 flex-col items-center justify-center gap-1 px-2 text-center"
					>
						<Button
							type="button"
							size="sm"
							variant="outline"
							class="header-identity-button h-7 max-w-full truncate px-2 text-base leading-snug font-medium transition-colors {selectedHeaderIdentity ===
							'user'
								? ''
								: 'text-muted-foreground'}"
							data-selected={selectedHeaderIdentity === 'user'}
							aria-pressed={selectedHeaderIdentity === 'user'}
							onclick={() => {
								selectedHeaderIdentity = 'user';
							}}
						>
							{getDisplayName(session)}
						</Button>
						<Button
							type="button"
							size="sm"
							variant="outline"
							class="header-identity-button h-7 max-w-full truncate px-2 text-base leading-snug font-medium transition-colors {selectedHeaderIdentity ===
							'role'
								? ''
								: 'text-muted-foreground'}"
							data-selected={selectedHeaderIdentity === 'role'}
							aria-pressed={selectedHeaderIdentity === 'role'}
							onclick={() => {
								selectedHeaderIdentity = 'role';
							}}
						>
							{activeAssistantName()}
						</Button>
					</div>
				</div>
			{/if}
		</CardHeader>
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
						<Input
							type="email"
							placeholder="Email"
							bind:value={email}
							autocomplete="email"
							required
						/>
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
			{:else if signedInMain === 'idle'}
				<div class="min-h-0 flex-1 shrink" aria-hidden="true"></div>
			{:else if signedInMain === 'chats'}
				<ScrollArea class="min-h-0 flex-1 rounded-md border border-input bg-field">
					<div class="min-h-full space-y-2 p-3">
						{#if chatsLoading}
							<p class="text-sm text-muted-foreground">Loading chats…</p>
						{/if}
						{#if chatsLoadError}
							<p class="text-sm text-destructive">{chatsLoadError}</p>
						{/if}
						<ul class="space-y-1.5">
							<li>
								<button
									type="button"
									class="w-full rounded-md border border-border bg-background/80 px-3 py-2.5 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
									onclick={handleNewChat}
								>
									New chat
								</button>
							</li>
							{#each chatRows as c (c.id)}
								{@const dateLabel = formatChatListDate(c.lastUpdated)}
								<li>
									<button
										type="button"
										class="w-full rounded-md border px-3 py-2.5 text-left text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline {chatId ===
										c.id
											? 'border-primary/50 bg-muted/40'
											: 'border-border bg-background/40 hover:bg-muted/50'}"
										onclick={() => void openChatFromList(c)}
									>
										<div class="font-medium text-foreground">{c.title.trim() || 'Untitled'}</div>
										{#if dateLabel}
											<div class="mt-0.5 text-xs text-muted-foreground">{dateLabel}</div>
										{/if}
									</button>
								</li>
							{/each}
						</ul>
						{#if !chatsLoading && !chatsLoadError && chatRows.length === 0}
							<p class="text-sm text-muted-foreground">No saved chats yet.</p>
						{/if}
					</div>
				</ScrollArea>
			{:else if signedInMain === 'roles'}
				<ScrollArea class="min-h-0 flex-1 rounded-md border border-input bg-field">
					<div class="min-h-full space-y-2 p-3">
						{#if rolesLoading}
							<p class="text-sm text-muted-foreground">Loading roles…</p>
						{/if}
						{#if rolesLoadError}
							<p class="text-sm text-destructive">{rolesLoadError}</p>
						{/if}
						{#if rolesUI === 'list'}
							<ul class="space-y-1.5">
								<li>
									<button
										type="button"
										class="w-full rounded-md border border-border bg-background/80 px-3 py-2.5 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
										onclick={openNewRoleEditor}
									>
										New role
									</button>
								</li>
								{#each roleRows as r (r.id)}
									<li class="flex items-center gap-1.5">
										<button
											type="button"
											class="min-w-0 flex-1 rounded-md border px-3 py-2.5 text-left text-sm text-foreground transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline {selectedRoleId ===
											r.id
												? 'border-primary/50 bg-muted/40'
												: 'border-border bg-background/40 hover:bg-muted/50'}"
											onclick={() => selectRoleAsCurrent(r)}
											aria-pressed={selectedRoleId === r.id}
											aria-label={`Use role ${r.name} for chat`}
										>
											{r.name}
										</button>
										<Button
											type="button"
											variant="outline"
											size="icon-sm"
											class="shrink-0"
											onclick={() => openExistingRoleEditor(r)}
											aria-label={`Edit role ${r.name}`}
										>
											<Feather class="size-4" aria-hidden="true" />
										</Button>
										<Button
											type="button"
											variant="outline"
											size="icon-sm"
											class="shrink-0"
											disabled={deletingRoleId === r.id}
											onclick={(e: MouseEvent) => deleteRoleRow(r, e)}
											aria-label={`Delete role ${r.name}`}
										>
											<Trash2 class="size-4" aria-hidden="true" />
										</Button>
									</li>
								{/each}
							</ul>
						{:else}
							<div class="flex flex-col gap-3">
								<Button
									type="button"
									variant="ghost"
									size="sm"
									class="w-fit gap-1.5 px-2"
									onclick={openRoleList}
								>
									<ArrowLeft class="size-4" aria-hidden="true" />
									All roles
								</Button>
								<label class="sr-only" for="role-description-editor">Role description</label>
								<textarea
									id="role-description-editor"
									class="min-h-[14rem] w-full resize-y rounded-md border border-input bg-field px-2.5 py-2 text-sm shadow-xs transition-[color,box-shadow] outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50"
									placeholder={detailIsNew
										? 'Describe how this role should behave, including tone, expertise, and boundaries…'
										: 'Edit the description for this role…'}
									bind:value={detailDescription}
									disabled={savingRole}
									rows={10}
								></textarea>
							</div>
						{/if}
					</div>
				</ScrollArea>
				{#if roleSaveError}
					<p class="text-sm text-destructive">{roleSaveError}</p>
				{/if}
				<form class="flex gap-2" onsubmit={handleRoleSubmit}>
					{#if rolesUI === 'list'}
						<Input type="text" value="" disabled placeholder="" class="flex-1" />
					{:else}
						<Input
							type="text"
							bind:value={detailName}
							readonly={!detailIsNew}
							placeholder={detailIsNew ? 'Role name' : ''}
							disabled={savingRole}
							autocomplete="off"
							class="flex-1"
						/>
					{/if}
					<Button
						type="submit"
						disabled={roleSendDisabled()}
						aria-label={rolesUI === 'list' ? 'Save disabled' : 'Save role'}
					>
						<SendHorizontal class="size-4" />
					</Button>
				</form>
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
													class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-headings:first:mt-0 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-strong:font-semibold prose-strong:text-foreground prose-pre:my-2"
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
																class="reasoning-disclosure flex w-full items-center justify-start gap-0.5 border-b border-transparent px-2 py-1.5 text-left transition-colors hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
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
																class="reasoning-disclosure flex w-full items-center justify-start gap-0.5 border-b border-border/50 px-2 py-1.5 text-left transition-colors hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
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
																class="max-h-64 overflow-y-auto px-2 py-2 font-mono text-xs leading-relaxed break-words whitespace-pre-wrap text-muted-foreground"
															>
																{msg.reasoning?.trim() ? msg.reasoning : 'Waiting for reasoning…'}
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
													<span
														class="inline-flex items-baseline font-normal text-muted-foreground"
													>
														<span>({msg.sender_model ?? 'assistant'})</span>
														{#if showAssistantModelThinking(i, msg)}
															<span
																class="thinking-dots ml-1 text-muted-foreground"
																aria-hidden="true"><span>.</span><span>.</span><span>.</span></span
															>
														{/if}
													</span>
												</div>
												{#if prefix?.rest?.trim()}
													<div
														class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-headings:first:mt-0 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-strong:font-semibold prose-strong:text-foreground prose-pre:my-2"
													>
														<!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify sanitizes LLM HTML -->
														{@html renderMarkdown(prefix.rest)}
													</div>
												{:else if !prefix && msg.content.trim()}
													<div
														class="prose prose-sm max-w-none text-foreground dark:prose-invert prose-headings:mt-3 prose-headings:mb-2 prose-headings:first:mt-0 prose-p:my-2 first:prose-p:mt-0 last:prose-p:mb-0 prose-strong:font-semibold prose-strong:text-foreground prose-pre:my-2"
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
	{#if headerHoverTip}
		<div
			class="pointer-events-none fixed z-[200]"
			style:left="{headerHoverTip.x}px"
			style:top="{headerHoverTip.y}px"
			style:transform="translateX(-50%)"
		>
			<div
				class="max-w-xs rounded-md border border-border bg-[color-mix(in_oklch,var(--card)_78%,var(--foreground)_22%)] px-2.5 py-1.5 text-xs text-card-foreground shadow-card ring-1 ring-black/10 dark:bg-[color-mix(in_oklch,var(--card)_72%,black_28%)] dark:ring-white/15"
				transition:fade={{ delay: 55, duration: 145 }}
				role="tooltip"
			>
				{headerHoverTip.text}
			</div>
		</div>
	{/if}
</main>

<style>
	:global(.chat-subtitle-clamp) {
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		overflow: hidden;
	}

	:global(html:not(.dark) .header-control-btn:hover) {
		background-color: color-mix(in oklch, var(--primary) 28%, var(--background));
		border-color: color-mix(in oklch, var(--primary) 62%, var(--border));
		color: color-mix(in oklch, var(--foreground) 78%, var(--primary));
		box-shadow:
			0 0 0 1px color-mix(in oklch, var(--primary) 34%, transparent),
			0 6px 14px -8px color-mix(in oklch, var(--primary) 38%, transparent);
		transform: translateY(-1px);
	}
</style>
