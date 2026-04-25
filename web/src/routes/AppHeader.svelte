<script lang="ts">
	import { fade, fly } from 'svelte/transition';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { CardDescription, CardHeader, CardTitle } from '$lib/components/ui/card';
	import GalleryVerticalEnd from '@lucide/svelte/icons/gallery-vertical-end';
	import MessagesSquare from '@lucide/svelte/icons/messages-square';
	import Plus from '@lucide/svelte/icons/plus';
	import Moon from '@lucide/svelte/icons/moon';
	import Sun from '@lucide/svelte/icons/sun';
	import Menu from '@lucide/svelte/icons/menu';
	import Drama from '@lucide/svelte/icons/drama';
	import X from '@lucide/svelte/icons/x';

	let {
		authState,
		signedInMain,
		sessionSubtitle,
		onNewChat,
		onChatGallery,
		onCurrentChat,
		onToggleTheme,
		onSignOut
	}: {
		authState: 'loading' | 'signed_out' | 'signed_in';
		signedInMain: 'chat' | 'chats';
		sessionSubtitle: string;
		onNewChat: () => void;
		onChatGallery: () => void;
		onCurrentChat: () => void;
		onToggleTheme: () => void;
		onSignOut: () => void;
	} = $props();

	let headerHoverTip = $state<{ text: string; x: number; y: number } | null>(null);
	let mobileMenuOpen = $state(false);
	let mobileRolesOpen = $state(false);

	function showHeaderHoverTip(text: string, e: PointerEvent) {
		const el = e.currentTarget;
		if (!(el instanceof HTMLElement)) return;
		const r = el.getBoundingClientRect();
		headerHoverTip = { text, x: r.left + r.width / 2, y: r.bottom + 6 };
	}

	function hideHeaderHoverTip() {
		headerHoverTip = null;
	}

	function toggleMobileMenu() {
		const next = !mobileMenuOpen;
		mobileMenuOpen = next;
		if (next) mobileRolesOpen = false;
	}

	function toggleMobileRoles() {
		const next = !mobileRolesOpen;
		mobileRolesOpen = next;
		if (next) mobileMenuOpen = false;
	}

	function closeMobileMenu() {
		mobileMenuOpen = false;
	}

	function closeMobileRoles() {
		mobileRolesOpen = false;
	}

	function runMenuAction(action: () => void) {
		action();
		closeMobileMenu();
	}
</script>

<CardHeader class="relative flex shrink-0 flex-col items-stretch gap-2 space-y-0">
	<div class="flex min-w-0 items-start gap-2 md:items-center">
		<div class="w-fit max-w-[20rem] min-w-0 shrink space-y-1">
			<CardTitle class="text-2xl text-primary">Token Soup</CardTitle>
			<CardDescription class="text-base break-words truncate"
				>{signedInMain === 'chats' ? 'Chats' : sessionSubtitle}</CardDescription
			>
		</div>
		<div class="min-w-0 flex-1" aria-hidden="true"></div>
		<Button
			type="button"
			variant="outline"
			size="icon-sm"
			class="header-control-btn shrink-0 md:hidden"
			onclick={toggleMobileMenu}
			aria-label={mobileMenuOpen ? 'Close menu' : 'Open menu'}
			aria-haspopup="menu"
			aria-expanded={mobileMenuOpen}
			aria-controls="mobile-header-menu"
		>
			<Menu class="size-4" />
		</Button>
		<Button
			type="button"
			variant="outline"
			size="icon-sm"
			class="header-control-btn shrink-0 md:hidden"
			onclick={toggleMobileRoles}
			aria-label={mobileRolesOpen ? 'Close roles' : 'Open roles'}
			aria-haspopup="menu"
			aria-expanded={mobileRolesOpen}
			aria-controls="mobile-roles-menu"
		>
			<Drama class="size-4" />
		</Button>
		<div class="hidden shrink-0 items-center gap-2 md:ml-auto md:flex">
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
	</div>
	{#if mobileMenuOpen}
		<div class="fixed inset-0 z-[190] bg-black/45 md:hidden" onclick={closeMobileMenu}></div>
		<aside
			id="mobile-header-menu"
			class="fixed right-0 top-0 z-[200] flex h-screen w-[18rem] flex-col gap-3 border-l border-border bg-card p-3 shadow-card md:hidden"
			role="menu"
			transition:fly={{ x: 20, duration: 170 }}
		>
			<div class="flex items-center justify-between">
				<p class="text-sm font-medium text-muted-foreground">Menu</p>
				<Button
					type="button"
					variant="outline"
					size="icon-sm"
					class="header-control-btn shrink-0"
					onclick={closeMobileMenu}
					aria-label="Close menu"
				>
					<X class="size-4" />
				</Button>
			</div>
			<div class="flex flex-col gap-2">
				<Button
					type="button"
					variant="outline"
					size="sm"
					class="header-control-btn justify-start gap-2"
					onclick={() => runMenuAction(onNewChat)}
					aria-label="New chat"
				>
					<Plus class="size-4" />
					<span>New chat</span>
				</Button>
				<Button
					type="button"
					variant={signedInMain === 'chats' ? 'secondary' : 'outline'}
					size="sm"
					class="header-control-btn justify-start gap-2"
					onclick={() => runMenuAction(onChatGallery)}
					aria-label="Chats"
					aria-pressed={signedInMain === 'chats'}
				>
					<GalleryVerticalEnd class="size-4" />
					<span>Chats</span>
				</Button>
				<Button
					type="button"
					variant={signedInMain === 'chat' ? 'secondary' : 'outline'}
					size="sm"
					class="header-control-btn justify-start gap-2"
					onclick={() => runMenuAction(onCurrentChat)}
					aria-label="Current chat"
					aria-pressed={signedInMain === 'chat'}
				>
					<MessagesSquare class="size-4" />
					<span>Current chat</span>
				</Button>
				<Button
					type="button"
					variant="outline"
					size="sm"
					class="header-control-btn justify-start gap-2"
					onclick={() => runMenuAction(onToggleTheme)}
					aria-label="Toggle color theme"
				>
					<Sun class="hidden size-4 dark:block" />
					<Moon class="size-4 dark:hidden" />
					<span>Theme</span>
				</Button>
			</div>
			{#if authState === 'signed_in'}
				<Button
					type="button"
					variant="outline"
					size="sm"
					class="header-control-btn justify-start gap-2"
					onclick={() => runMenuAction(onSignOut)}
				>
					<span>Sign out</span>
				</Button>
			{:else if authState === 'signed_out'}
				<Badge variant="secondary">Signed out</Badge>
			{:else}
				<Badge variant="secondary">…</Badge>
			{/if}
			<div class="min-h-0 flex-1 overflow-y-auto">
				<slot name="mobile-sidebar-content" />
			</div>
		</aside>
	{/if}
	{#if mobileRolesOpen}
		<div class="fixed inset-0 z-[190] bg-black/45 md:hidden" onclick={closeMobileRoles}></div>
		<aside
			id="mobile-roles-menu"
			class="fixed right-0 top-0 z-[200] flex h-screen w-[18rem] flex-col gap-3 border-l border-border bg-card p-3 shadow-card md:hidden"
			role="menu"
			transition:fly={{ x: 20, duration: 170 }}
		>
			<div class="flex items-center justify-between">
				<p class="text-sm font-medium text-muted-foreground">Roles</p>
				<Button
					type="button"
					variant="outline"
					size="icon-sm"
					class="header-control-btn shrink-0"
					onclick={closeMobileRoles}
					aria-label="Close roles"
				>
					<X class="size-4" />
				</Button>
			</div>
			<div class="min-h-0 flex-1 overflow-y-auto">
				<slot name="mobile-roles-sidebar-content" />
			</div>
		</aside>
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
	:global(html:not(.dark) .header-control-btn:hover) {
		background-color: color-mix(in oklch, var(--primary) 28%, var(--background));
		border-color: color-mix(in oklch, var(--primary) 62%, var(--border));
		color: color-mix(in oklch, var(--foreground) 78%, var(--primary));
	}
</style>
