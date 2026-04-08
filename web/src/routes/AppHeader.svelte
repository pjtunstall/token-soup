<script lang="ts">
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import Drama from '@lucide/svelte/icons/drama';
	import GalleryVerticalEnd from '@lucide/svelte/icons/gallery-vertical-end';
	import MessagesSquare from '@lucide/svelte/icons/messages-square';
	import Plus from '@lucide/svelte/icons/plus';
	import Moon from '@lucide/svelte/icons/moon';
	import Sun from '@lucide/svelte/icons/sun';

	let {
		authState,
		signedInMain,
		sessionSubtitle,
		displayName,
		assistantName,
		selectedHeaderIdentity,
		onNewChat,
		onChatGallery,
		onCurrentChat,
		onRoles,
		onToggleTheme,
		onSignOut,
		onSelectIdentity
	}: {
		authState: 'loading' | 'signed_out' | 'signed_in';
		signedInMain: 'chat' | 'chats' | 'idle' | 'roles';
		sessionSubtitle: string;
		displayName: string;
		assistantName: string;
		selectedHeaderIdentity: 'user' | 'role';
		onNewChat: () => void;
		onChatGallery: () => void;
		onCurrentChat: () => void;
		onRoles: () => void;
		onToggleTheme: () => void;
		onSignOut: () => void;
		onSelectIdentity: (id: 'user' | 'role') => void;
	} = $props();

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
</script>

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
			onclick={onNewChat}
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
			onclick={onChatGallery}
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
			onclick={onCurrentChat}
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
			onclick={onRoles}
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
			onclick={onToggleTheme}
			aria-label="Toggle color theme"
		>
			<Sun class="hidden size-4 dark:block" />
			<Moon class="size-4 dark:hidden" />
		</Button>
		{#if authState === 'signed_in'}
			<Button
				type="button"
				variant="outline"
				size="sm"
				class="header-control-btn"
				onclick={onSignOut}>Sign out</Button
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
					class="header-control-btn header-identity-button h-7 max-w-full truncate px-2 text-base leading-snug font-medium transition-colors {selectedHeaderIdentity ===
					'user'
						? ''
						: 'text-muted-foreground'}"
					data-selected={selectedHeaderIdentity === 'user'}
					aria-pressed={selectedHeaderIdentity === 'user'}
					onclick={() => onSelectIdentity('user')}
				>
					{displayName}
				</Button>
				<Button
					type="button"
					size="sm"
					variant="outline"
					class="header-control-btn header-identity-button h-7 max-w-full truncate px-2 text-base leading-snug font-medium transition-colors {selectedHeaderIdentity ===
					'role'
						? ''
						: 'text-muted-foreground'}"
					data-selected={selectedHeaderIdentity === 'role'}
					aria-pressed={selectedHeaderIdentity === 'role'}
					onclick={() => onSelectIdentity('role')}
				>
					{assistantName}
				</Button>
			</div>
		</div>
	{/if}
</CardHeader>

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
	}
</style>
