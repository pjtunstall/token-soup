<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import { clampRoleForDisplay } from '$lib/display-clamp';
	import { ROLE_DESCRIPTION_MAX_CHARS, ROLE_NAME_MAX_CHARS } from '$lib/limits';
	import { createRole, updateRoleDescription, roleNameExists, type RoleRow } from '$lib/roles';
	import Feather from '@lucide/svelte/icons/feather';
	import Trash2 from '@lucide/svelte/icons/trash-2';

	let {
		roleRows,
		rolesLoading,
		rolesLoadingMore,
		rolesLoadError,
		rolesHasMore,
		selectedRoleId,
		deletingRoleId,
		displayName,
		userTurn,
		accessToken,
		onSelectRole,
		onSelectUser,
		onDoubleClickRole,
		onRolesChanged,
		onDeleteRole,
		onLoadMore
	}: {
		roleRows: RoleRow[];
		rolesLoading: boolean;
		rolesLoadingMore: boolean;
		rolesLoadError: string;
		rolesHasMore: boolean;
		selectedRoleId: number | null;
		deletingRoleId: number | null;
		displayName: string;
		userTurn: boolean;
		accessToken: string;
		onSelectRole: (r: RoleRow) => void;
		onSelectUser: () => void;
		onDoubleClickRole: (r: RoleRow) => void;
		onRolesChanged: () => Promise<void>;
		onDeleteRole: (r: RoleRow, e: MouseEvent) => void;
		onLoadMore: () => void;
	} = $props();

	let editorOpen = $state(false);
	let editorIsNew = $state(false);
	let editorRoleId = $state<number | null>(null);
	let editorName = $state('');
	let editorDescription = $state('');
	let editorSaving = $state(false);
	let editorError = $state('');
	let viewportRef = $state<HTMLElement | null>(null);
	const LOAD_MORE_PX = 240;

	export function closeEditor() {
		editorOpen = false;
		editorIsNew = false;
		editorRoleId = null;
		editorName = '';
		editorDescription = '';
		editorError = '';
		editorSaving = false;
	}

	function openNewRoleEditor() {
		editorOpen = true;
		editorIsNew = true;
		editorRoleId = null;
		editorName = '';
		editorDescription = '';
		editorError = '';
	}

	function openExistingRoleEditor(r: RoleRow) {
		editorOpen = true;
		editorIsNew = false;
		editorRoleId = r.id;
		editorName = r.name;
		editorDescription = r.description ?? '';
		editorError = '';
	}

	async function saveEditor() {
		if (!accessToken || !editorOpen || editorSaving) return;
		editorError = '';
		const desc = editorDescription.trim();
		if (!desc) {
			editorError = 'Role description is required.';
			return;
		}
		if ([...desc].length > ROLE_DESCRIPTION_MAX_CHARS) {
			editorError = `Role description must be ${ROLE_DESCRIPTION_MAX_CHARS} characters or fewer.`;
			return;
		}
		editorSaving = true;
		try {
			if (editorIsNew) {
				const name = editorName.trim();
				if (!name) {
					editorError = 'Role name is required.';
					return;
				}
				if ([...name].length > ROLE_NAME_MAX_CHARS) {
					editorError = `Role name must be ${ROLE_NAME_MAX_CHARS} characters or fewer.`;
					return;
				}
				const exists = await roleNameExists(accessToken, name);
				if (exists) {
					editorError = `Name '${name}' already exists`;
					return;
				}
				await createRole(accessToken, name, desc);
			} else if (editorRoleId != null) {
				await updateRoleDescription(accessToken, editorRoleId, desc);
			}
			await onRolesChanged();
			closeEditor();
		} catch (e) {
			const message = e instanceof Error ? e.message : String(e);
			if (message.toLowerCase().includes('already exists')) {
				const attemptedName = editorName.trim();
				editorError = attemptedName
					? `Name '${attemptedName}' already exists`
					: 'Role name already exists.';
			} else {
				editorError = message;
			}
		} finally {
			editorSaving = false;
		}
	}

	function maybeLoadMoreRoles() {
		if (!viewportRef || rolesLoading || rolesLoadingMore || !rolesHasMore) return;
		const remaining = viewportRef.scrollHeight - viewportRef.scrollTop - viewportRef.clientHeight;
		if (remaining <= LOAD_MORE_PX) onLoadMore();
	}

	$effect(() => {
		if (!viewportRef) return;
		const onScroll = () => maybeLoadMoreRoles();
		viewportRef.addEventListener('scroll', onScroll, { passive: true });
		return () => viewportRef?.removeEventListener('scroll', onScroll);
	});

	$effect(() => {
		void roleRows.length;
		maybeLoadMoreRoles();
	});
</script>

<p class="mb-2 shrink-0 px-0.5 text-sm text-muted-foreground">Pick who speaks next...</p>

<button
	type="button"
	class="menu-action-btn user-btn h-8 w-full truncate rounded-md border px-2 text-left text-sm transition-colors {userTurn
		? 'border-primary/55'
		: 'border-border bg-background/40 dark:border-input dark:bg-input/30'}"
	aria-label={editorOpen ? 'Editing role' : 'Signed in user'}
	aria-pressed={userTurn}
	data-selected={userTurn ? 'true' : undefined}
	onclick={() => {
		closeEditor();
		onSelectUser();
	}}
>
	{editorOpen ? editorName || 'New role' : displayName}
</button>

<div class="mt-2 min-h-0 flex-1 md:mt-0">
	{#if editorOpen}
		<div class="flex h-full min-h-0 flex-col gap-2">
			{#if editorIsNew}
				<div class="shrink-0">
					<div class="relative">
						<Input
							type="text"
							bind:value={editorName}
							oninput={() => {
								editorError = '';
							}}
							placeholder="Role name"
							maxlength={ROLE_NAME_MAX_CHARS}
							disabled={editorSaving}
							autocomplete="off"
							class="h-8 pr-16 text-sm"
						/>
						<p
							class="pointer-events-none absolute right-2 bottom-1 text-[11px] text-muted-foreground/90 dark:text-muted-foreground"
						>
							{[...editorName.trim()].length}/{ROLE_NAME_MAX_CHARS}
						</p>
					</div>
				</div>
			{/if}
			<div class="min-h-0 flex-1">
				<div class="relative h-full min-h-[10rem]">
					<textarea
						class="role-scrollbar h-full min-h-[10rem] w-full resize-none rounded-md border border-input bg-field px-2 py-1.5 pb-6 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
						placeholder={editorIsNew
							? 'Describe how this character should behave...'
							: 'Edit role description...'}
						bind:value={editorDescription}
						maxlength={ROLE_DESCRIPTION_MAX_CHARS}
						oninput={() => {
							editorError = '';
						}}
						disabled={editorSaving}
					></textarea>
					<p
						class="pointer-events-none absolute right-2 bottom-1.5 text-[11px] text-muted-foreground/90 dark:text-muted-foreground"
					>
						{[...editorDescription.trim()].length}/{ROLE_DESCRIPTION_MAX_CHARS}
					</p>
				</div>
				{#if editorError}
					<p class="mt-2 text-xs text-destructive">{editorError}</p>
				{/if}
			</div>
			<div class="shrink-0">
				<div class="flex gap-2">
					<Button
						type="button"
						variant="outline"
						size="sm"
						class="header-control-btn flex-1"
						onclick={closeEditor}
					>
						Exit
					</Button>
					<Button
						type="button"
						size="sm"
						class="flex-1"
						disabled={editorSaving ||
							(editorIsNew && !editorName.trim()) ||
							!editorDescription.trim()}
						onclick={() => void saveEditor()}
					>
						Save
					</Button>
				</div>
			</div>
		</div>
	{:else}
		<ScrollArea class="h-full min-h-0" bind:viewportRef>
			<div class="pr-2">
				{#if rolesLoading}
					<p class="mb-2 text-xs text-muted-foreground">Loading roles…</p>
				{/if}
				{#if rolesLoadError}
					<p class="mb-2 text-xs text-destructive">{rolesLoadError}</p>
				{/if}
				<Button
					type="button"
					variant="outline"
					size="sm"
					class="header-control-btn new-role-btn my-3 w-full justify-start font-semibold"
					onclick={openNewRoleEditor}
				>
					New role
				</Button>
				<ul class="space-y-1.5">
					{#each roleRows as r (r.id)}
						<li class="flex items-center gap-1">
							<button
								type="button"
								class="menu-action-btn role-option-btn min-w-0 flex-1 truncate rounded-md border px-2 py-1.5 text-left text-sm transition-colors {!userTurn &&
								selectedRoleId === r.id
									? 'border-primary/55'
									: 'border-border bg-background/40 dark:border-input dark:bg-input/30'}"
								onclick={() => {
									onSelectRole(r);
								}}
								ondblclick={() => {
									onDoubleClickRole(r);
								}}
								aria-pressed={!userTurn && selectedRoleId === r.id}
								data-selected={!userTurn && selectedRoleId === r.id ? 'true' : undefined}
							>
								{clampRoleForDisplay(r).name}
							</button>
							<Button
								type="button"
								variant="outline"
								size="icon-sm"
								class="menu-action-btn shrink-0"
								onclick={() => openExistingRoleEditor(r)}
								aria-label={`Edit role ${clampRoleForDisplay(r).name}`}
							>
								<Feather class="size-4" aria-hidden="true" />
							</Button>
							<Button
								type="button"
								variant="outline"
								size="icon-sm"
								class="menu-action-btn shrink-0"
								disabled={deletingRoleId === r.id}
								onclick={(e: MouseEvent) => onDeleteRole(r, e)}
								aria-label={`Delete role ${clampRoleForDisplay(r).name}`}
							>
								<Trash2 class="size-4" aria-hidden="true" />
							</Button>
						</li>
					{/each}
				</ul>
				{#if rolesLoadingMore}
					<p class="mt-2 text-xs text-muted-foreground">Loading more roles…</p>
				{/if}
			</div>
		</ScrollArea>
	{/if}
</div>

<style>
	.role-scrollbar {
		scrollbar-width: thin;
		scrollbar-color: var(--border) transparent;
	}

	.role-scrollbar::-webkit-scrollbar {
		width: 10px;
	}

	.role-scrollbar::-webkit-scrollbar-track {
		background: transparent;
	}

	.role-scrollbar::-webkit-scrollbar-thumb {
		background: var(--border);
		border-radius: 9999px;
		border: 1px solid transparent;
		background-clip: content-box;
	}

	:global(html:not(.dark) .menu-action-btn:hover) {
		background-color: color-mix(in oklch, var(--primary) 22%, var(--background)) !important;
		border-color: color-mix(in oklch, var(--primary) 56%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 82%, var(--primary));
	}

	:global(
		html:not(.dark) .role-option-btn[data-selected='true'],
		html:not(.dark) .user-btn[data-selected='true']
	) {
		background-color: color-mix(in oklch, var(--primary) 22%, var(--background)) !important;
		border-color: color-mix(in oklch, var(--primary) 62%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 88%, var(--primary));
	}

	:global(html.dark .role-option-btn:hover, html.dark .user-btn:hover) {
		background-color: color-mix(in oklch, var(--card) 72%, black 28%) !important;
		border-color: color-mix(in oklch, var(--primary) 58%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 90%, var(--primary));
	}

	:global(
		html.dark .role-option-btn[data-selected='true'],
		html.dark .user-btn[data-selected='true']
	) {
		background-color: color-mix(in oklch, var(--card) 72%, black 28%) !important;
		border-color: color-mix(in oklch, var(--primary) 60%, var(--border)) !important;
		color: color-mix(in oklch, var(--foreground) 94%, var(--primary));
		box-shadow: inset 0 0 0 1px color-mix(in oklch, var(--primary) 34%, transparent);
	}

	:global(html.dark .new-role-btn) {
		background-color: color-mix(in oklch, var(--secondary) 62%, black 38%) !important;
		border-color: color-mix(in oklch, var(--secondary) 78%, var(--foreground) 22%) !important;
		color: color-mix(in oklch, var(--foreground) 96%, var(--secondary)) !important;
		box-shadow: inset 0 0 0 1px color-mix(in oklch, var(--secondary) 52%, transparent) !important;
	}

	:global(html.dark .new-role-btn:hover) {
		background-color: color-mix(in oklch, var(--accent) 100%, var(--card)) !important;
		border-color: color-mix(in oklch, var(--foreground) 58%, var(--accent)) !important;
		color: color-mix(in oklch, var(--foreground) 100%, var(--accent)) !important;
		box-shadow:
			inset 0 0 0 1px color-mix(in oklch, var(--foreground) 42%, var(--accent)),
			0 0 0 1px color-mix(in oklch, var(--accent) 35%, transparent) !important;
	}
</style>
