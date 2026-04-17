import type { RoleRow } from '$lib/roles';

export function selectedRoleStorageKey(userId: string): string {
	return `token-soup:selectedRoleId:${userId}`;
}

export function chooseSelectedRoleId(roleRows: RoleRow[], rawStoredId: string | null): number | null {
	let selectedRoleId: number | null = null;
	if (rawStoredId) {
		const parsed = Number(rawStoredId);
		if (Number.isFinite(parsed) && roleRows.some((row) => row.id === parsed)) {
			selectedRoleId = parsed;
		}
	}

	if (selectedRoleId == null) {
		const guru = roleRows.find((row) => row.name.toLowerCase() === 'guru');
		if (guru) selectedRoleId = guru.id;
		else if (roleRows.length > 0) selectedRoleId = roleRows[0]!.id;
	}

	return selectedRoleId;
}

export function activeAssistantName(roleRows: RoleRow[], selectedRoleId: number | null): string {
	if (selectedRoleId != null) {
		const row = roleRows.find((r) => r.id === selectedRoleId);
		if (row) return row.name;
	}
	return 'Guru';
}

export function getStoredSelectedRoleId(storage: Storage, userId: string): string | null {
	try {
		return storage.getItem(selectedRoleStorageKey(userId));
	} catch {
		return null;
	}
}

export function persistSelectedRoleId(storage: Storage, userId: string, roleId: number): void {
	try {
		storage.setItem(selectedRoleStorageKey(userId), String(roleId));
	} catch {
		/* ignore */
	}
}
