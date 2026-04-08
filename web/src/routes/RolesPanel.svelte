<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { createRole, updateRoleDescription, roleNameExists, type RoleRow } from '$lib/roles';
	import Feather from '@lucide/svelte/icons/feather';
	import Trash2 from '@lucide/svelte/icons/trash-2';

	let {
		roleRows,
		rolesLoading,
		rolesLoadError,
		selectedRoleId,
		deletingRoleId,
		displayName,
		accessToken,
		onSelectRole,
		onRolesChanged,
		onDeleteRole
	}: {
		roleRows: RoleRow[];
		rolesLoading: boolean;
		rolesLoadError: string;
		selectedRoleId: number | null;
		deletingRoleId: number | null;
		displayName: string;
		accessToken: string;
		onSelectRole: (r: RoleRow) => void;
		onRolesChanged: () => Promise<void>;
		onDeleteRole: (r: RoleRow, e: MouseEvent) => void;
	} = $props();

	let editorOpen = $state(false);
	let editorIsNew = $state(false);
	let editorRoleId = $state<number | null>(null);
	let editorName = $state('');
	let editorDescription = $state('');
	let editorSaving = $state(false);
	let editorError = $state('');

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
		editorSaving = true;
		try {
			if (editorIsNew) {
				const name = editorName.trim();
				if (!name) {
					editorError = 'Role name is required.';
					return;
				}
				const exists = await roleNameExists(accessToken, name);
				if (exists) {
					editorError = `Name '${name}' already exists`;
					return;
				}
				await createRole(accessToken, name, editorDescription.trim() || null);
			} else if (editorRoleId != null) {
				await updateRoleDescription(accessToken, editorRoleId, editorDescription.trim() || null);
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
</script>

<Button
	type="button"
	size="sm"
	variant="outline"
	class="header-control-btn h-8 w-full justify-start truncate px-2 text-sm disabled:opacity-100"
	disabled
	aria-label={editorOpen ? 'Editing role' : 'Signed in user'}
	onclick={closeEditor}
>
	{editorOpen ? editorName || 'New role' : displayName}
</Button>

<div class="min-h-0 flex-1">
	{#if editorOpen}
		<div class="flex h-full min-h-0 flex-col gap-2">
			{#if editorIsNew}
				<div class="shrink-0 space-y-2">
					<Input
						type="text"
						bind:value={editorName}
						oninput={() => {
							editorError = '';
						}}
						placeholder="Role name"
						disabled={editorSaving}
						autocomplete="off"
						class="h-8 text-sm"
					/>
					{#if editorError}
						<p class="text-sm text-destructive">{editorError}</p>
					{/if}
				</div>
			{/if}
			<div class="min-h-0 flex-1">
				<textarea
					class="h-full min-h-[10rem] w-full resize-none rounded-md border border-input bg-field px-2 py-1.5 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
					placeholder={editorIsNew
						? 'Describe how this role should behave...'
						: 'Edit role description...'}
					bind:value={editorDescription}
					disabled={editorSaving}
				></textarea>
				{#if editorError && !editorIsNew}
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
						disabled={editorSaving || (editorIsNew && !editorName.trim())}
						onclick={() => void saveEditor()}
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
								onSelectRole(r);
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
							onclick={(e: MouseEvent) => onDeleteRole(r, e)}
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
