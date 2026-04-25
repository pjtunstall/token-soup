import { deleteRole, fetchRoles, type RoleRow } from '$lib/roles';
import { loadFirstPage, loadNextPage } from '$lib/pagination';
import {
	activeAssistantName,
	chooseSelectedRoleId,
	getStoredSelectedRoleId,
	persistSelectedRoleId
} from '$lib/role-selection';

export type RolesControllerOptions = {
	getAccessToken: () => string | undefined;
	getUserId: () => string | undefined;
	pageSize: number;
	getStorage: () => Storage;
	mapError?: (err: unknown) => string;
};

export class RolesController {
	rows = $state<RoleRow[]>([]);
	loading = $state(false);
	loadingMore = $state(false);
	hasMore = $state(true);
	loadError = $state('');
	deletingId = $state<number | null>(null);
	/** Persisted AI persona (`public.roles`); drives system prompt and assistant label. */
	selectedId = $state<number | null>(null);
	/** When true the user is typing; when false the selected AI role will generate next. */
	userTurn = $state(false);

	private readonly opts: RolesControllerOptions;

	constructor(opts: RolesControllerOptions) {
		this.opts = opts;
	}

	get assistantName(): string {
		return activeAssistantName(this.rows, this.selectedId);
	}

	async load(): Promise<void> {
		const accessToken = this.opts.getAccessToken();
		if (!accessToken) return;
		this.loading = true;
		this.loadError = '';
		try {
			const page = await loadFirstPage(
				(params) => fetchRoles(accessToken, params),
				this.opts.pageSize
			);
			this.rows = page.rows;
			this.hasMore = page.hasMore;
		} catch (e) {
			this.rows = [];
			this.hasMore = true;
			this.loadError = this.formatError(e);
		} finally {
			this.loading = false;
		}
		this.syncSelectedAfterLoad();
	}

	async loadMore(): Promise<void> {
		const accessToken = this.opts.getAccessToken();
		if (!accessToken || this.loading || this.loadingMore || !this.hasMore) return;
		this.loadingMore = true;
		this.loadError = '';
		try {
			const page = await loadNextPage(
				(params) => fetchRoles(accessToken, params),
				this.opts.pageSize,
				this.rows
			);
			this.rows = page.rows;
			this.hasMore = page.hasMore;
		} catch (e) {
			this.loadError = this.formatError(e);
		} finally {
			this.loadingMore = false;
		}
	}

	async deleteRow(r: RoleRow): Promise<void> {
		const accessToken = this.opts.getAccessToken();
		if (!accessToken || this.deletingId != null) return;
		this.deletingId = r.id;
		this.loadError = '';
		try {
			await deleteRole(accessToken, r.id);
			await this.load();
		} catch (err) {
			this.loadError = this.formatError(err);
		} finally {
			this.deletingId = null;
		}
	}

	selectRole(r: RoleRow): void {
		this.selectedId = r.id;
		this.userTurn = false;
		const userId = this.opts.getUserId();
		if (userId) {
			persistSelectedRoleId(this.opts.getStorage(), userId, r.id);
		}
	}

	selectUser(): void {
		this.userTurn = true;
	}

	reset(): void {
		this.rows = [];
		this.loadError = '';
		this.hasMore = true;
		this.loadingMore = false;
		this.selectedId = null;
		this.userTurn = false;
	}

	private syncSelectedAfterLoad(): void {
		const userId = this.opts.getUserId();
		if (!userId) return;
		const storage = this.opts.getStorage();
		const rawStoredId = getStoredSelectedRoleId(storage, userId);
		const picked = chooseSelectedRoleId(this.rows, rawStoredId);
		this.selectedId = picked;
		if (picked != null) {
			persistSelectedRoleId(storage, userId, picked);
		}
	}

	private formatError(err: unknown): string {
		const message = err instanceof Error ? err.message : String(err);
		return this.opts.mapError ? this.opts.mapError(err) : message;
	}
}
