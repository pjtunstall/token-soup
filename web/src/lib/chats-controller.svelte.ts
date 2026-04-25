import { fetchChat, fetchChats, type ChatSummaryRow } from '$lib/chats';
import { loadFirstPage, loadNextPage } from '$lib/pagination';
import type { ChatMessage } from '$lib/chat';

export type ChatDetail = { id: number; title: string; messages: ChatMessage[] };

export type ChatsControllerOptions = {
	getAccessToken: () => string | undefined;
	pageSize: number;
};

export class ChatsController {
	rows = $state<ChatSummaryRow[]>([]);
	loading = $state(false);
	loadingMore = $state(false);
	hasMore = $state(true);
	loadError = $state('');

	private readonly opts: ChatsControllerOptions;

	constructor(opts: ChatsControllerOptions) {
		this.opts = opts;
	}

	async load(): Promise<void> {
		const accessToken = this.opts.getAccessToken();
		if (!accessToken) return;
		this.loading = true;
		this.loadError = '';
		try {
			const page = await loadFirstPage(
				(params) => fetchChats(accessToken, params),
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
	}

	async loadMore(): Promise<void> {
		const accessToken = this.opts.getAccessToken();
		if (!accessToken || this.loading || this.loadingMore || !this.hasMore) return;
		this.loadingMore = true;
		this.loadError = '';
		try {
			const page = await loadNextPage(
				(params) => fetchChats(accessToken, params),
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

	async fetchDetail(id: number): Promise<ChatDetail | null> {
		const accessToken = this.opts.getAccessToken();
		if (!accessToken) return null;
		try {
			return await fetchChat(accessToken, id);
		} catch (e) {
			this.loadError = this.formatError(e);
			return null;
		}
	}

	clearError(): void {
		this.loadError = '';
	}

	reset(): void {
		this.rows = [];
		this.loadError = '';
		this.hasMore = true;
		this.loadingMore = false;
	}

	private formatError(err: unknown): string {
		return err instanceof Error ? err.message : String(err);
	}
}
