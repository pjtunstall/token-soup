<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { ScrollArea } from '$lib/components/ui/scroll-area';
	import { createRole, updateRoleDescription, type RoleRow } from '$lib/roles';
	import ArrowLeft from '@lucide/svelte/icons/arrow-left';
	import Feather from '@lucide/svelte/icons/feather';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import SendHorizontal from '@lucide/svelte/icons/send-horizontal';

	let {
		roleRows,
		rolesLoading,
		rolesLoadError,
		selectedRoleId,
		deletingRoleId,
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
		accessToken: string;
		onSelectRole: (r: RoleRow) => void;
		onRolesChanged: () => void;
		onDeleteRole: (r: RoleRow, e: MouseEvent) => void;
	} = $props();

	let rolesUI = $state<'list' | 'detail'>('list');
	let detailIsNew = $state(false);
	let detailRoleId = $state<number | null>(null);
	let detailName = $state('');
	let detailDescription = $state('');
	let savingRole = $state(false);
	let roleSaveError = $state('');

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

	function sendDisabled(): boolean {
		if (rolesUI === 'list') return true;
		if (savingRole) return true;
		if (detailIsNew) return !detailName.trim();
		return false;
	}

	async function saveRoleFromEditor() {
		if (!accessToken || savingRole || rolesUI !== 'detail') return;
		roleSaveError = '';
		savingRole = true;
		try {
			if (detailIsNew) {
				const name = detailName.trim();
				if (!name) {
					savingRole = false;
					return;
				}
				await createRole(accessToken, name, detailDescription.trim() || null);
			} else if (detailRoleId != null) {
				await updateRoleDescription(
					accessToken,
					detailRoleId,
					detailDescription.trim() || null
				);
			}
			onRolesChanged();
			openRoleList();
		} catch (e) {
			roleSaveError = e instanceof Error ? e.message : String(e);
		} finally {
			savingRole = false;
		}
	}

	function handleDeleteRole(r: RoleRow, e: MouseEvent) {
		if (detailRoleId === r.id) {
			openRoleList();
		}
		onDeleteRole(r, e);
	}

	function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		if (sendDisabled()) return;
		void saveRoleFromEditor();
	}

	export function resetToList() {
		openRoleList();
	}
</script>

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
						class="menu-action-btn w-full rounded-md border border-border bg-background/80 px-3 py-2.5 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline"
						onclick={openNewRoleEditor}
					>
						New role
					</button>
				</li>
				{#each roleRows as r (r.id)}
					<li class="flex items-center gap-1.5">
						<button
							type="button"
							class="menu-action-btn min-w-0 flex-1 rounded-md border px-3 py-2.5 text-left text-sm text-foreground transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline {selectedRoleId ===
							r.id
								? 'border-primary/50 bg-muted/40'
								: 'border-border bg-background/40 hover:bg-muted/50'}"
							onclick={() => onSelectRole(r)}
							aria-pressed={selectedRoleId === r.id}
							aria-label={`Use role ${r.name} for chat`}
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
							onclick={(e: MouseEvent) => handleDeleteRole(r, e)}
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
					class="menu-action-btn w-fit gap-1.5 px-2"
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
<form class="flex gap-2" onsubmit={handleSubmit}>
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
		disabled={sendDisabled()}
		aria-label={rolesUI === 'list' ? 'Save disabled' : 'Save role'}
	>
		<SendHorizontal class="size-4" />
	</Button>
</form>
