type PageFetcher<T> = (params: { limit: number; offset: number }) => Promise<T[]>;

export async function loadFirstPage<T>(
	fetchPage: PageFetcher<T>,
	pageSize: number
): Promise<{ rows: T[]; hasMore: boolean }> {
	const rows = await fetchPage({ limit: pageSize, offset: 0 });
	return { rows, hasMore: rows.length === pageSize };
}

export async function loadNextPage<T>(
	fetchPage: PageFetcher<T>,
	pageSize: number,
	existingRows: T[]
): Promise<{ rows: T[]; hasMore: boolean }> {
	const nextPage = await fetchPage({ limit: pageSize, offset: existingRows.length });
	return {
		rows: [...existingRows, ...nextPage],
		hasMore: nextPage.length === pageSize
	};
}
