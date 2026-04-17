import { PUBLIC_API_URL } from '$env/static/public';

export type RoleRow = { id: number; name: string; description: string | null };
type ListPageParams = { limit?: number; offset?: number };

/** Empty string = same origin (dev: Vite proxies `/api` to the Go API). Matches `$lib/chat` apiBase. */
function apiBase(): string {
	const raw = (PUBLIC_API_URL ?? '').trim();
	if (!raw) return '';
	return raw.replace(/\/$/, '');
}

function authHeaders(accessToken: string): HeadersInit {
	return {
		Authorization: `Bearer ${accessToken}`,
		'Content-Type': 'application/json'
	};
}

/** GET /api/roles */
export async function fetchRoles(
	accessToken: string,
	{ limit = 10, offset = 0 }: ListPageParams = {}
): Promise<RoleRow[]> {
	const base = apiBase();
	const query = new URLSearchParams({
		limit: String(limit),
		offset: String(offset)
	});
	const res = await fetch(`${base}/api/roles?${query.toString()}`, {
		method: 'GET',
		headers: authHeaders(accessToken)
	});
	if (res.status === 503) {
		throw new Error('Database is not available on the server.');
	}
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(text || `could not load roles (${res.status})`);
	}
	return (await res.json()) as RoleRow[];
}

/** POST /api/roles */
export async function createRole(
	accessToken: string,
	name: string,
	description: string | null
): Promise<RoleRow> {
	const base = apiBase();
	const res = await fetch(`${base}/api/roles`, {
		method: 'POST',
		headers: authHeaders(accessToken),
		body: JSON.stringify({
			name,
			description: description === '' || description == null ? null : description
		})
	});
	if (res.status === 503) {
		throw new Error('Database is not available on the server.');
	}
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(text || `could not create role (${res.status})`);
	}
	return (await res.json()) as RoleRow;
}

/** GET /api/roles + local match check for name uniqueness. */
export async function roleNameExists(accessToken: string, name: string): Promise<boolean> {
	const normalized = name.trim().toLowerCase();
	if (!normalized) return false;
	const pageSize = 100;
	let offset = 0;
	while (true) {
		const roles = await fetchRoles(accessToken, { limit: pageSize, offset });
		if (roles.some((r) => r.name.trim().toLowerCase() === normalized)) return true;
		if (roles.length < pageSize) return false;
		offset += pageSize;
	}
}

/** PATCH /api/roles/:id */
export async function updateRoleDescription(
	accessToken: string,
	id: number,
	description: string | null
): Promise<void> {
	const base = apiBase();
	const res = await fetch(`${base}/api/roles/${id}`, {
		method: 'PATCH',
		headers: authHeaders(accessToken),
		body: JSON.stringify({
			description: description === '' || description == null ? null : description
		})
	});
	if (res.status === 503) {
		throw new Error('Database is not available on the server.');
	}
	if (res.status === 404) {
		throw new Error('Role not found.');
	}
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(text || `could not update role (${res.status})`);
	}
}

/** DELETE /api/roles/:id */
export async function deleteRole(accessToken: string, id: number): Promise<void> {
	const base = apiBase();
	const res = await fetch(`${base}/api/roles/${id}`, {
		method: 'DELETE',
		headers: { Authorization: `Bearer ${accessToken}` }
	});
	if (res.status === 503) {
		throw new Error('Database is not available on the server.');
	}
	if (res.status === 404) {
		throw new Error('Role not found.');
	}
	if (!res.ok) {
		const text = await res.text().catch(() => '');
		throw new Error(text || `could not delete role (${res.status})`);
	}
}
