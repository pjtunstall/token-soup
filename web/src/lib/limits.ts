import { PUBLIC_ROLE_NAME_MAX_CHARS, PUBLIC_USER_MESSAGE_MAX_CHARS } from '$env/static/public';

const DEFAULT_ROLE_NAME_MAX_CHARS = 64;
const DEFAULT_USER_MESSAGE_MAX_CHARS = 12000;

function parsePositiveInt(v: string | undefined, fallback: number): number {
	if (!v) return fallback;
	const n = Number.parseInt(v, 10);
	if (!Number.isFinite(n) || n < 1) return fallback;
	return n;
}

export const ROLE_NAME_MAX_CHARS = parsePositiveInt(
	PUBLIC_ROLE_NAME_MAX_CHARS,
	DEFAULT_ROLE_NAME_MAX_CHARS
);
export const USER_MESSAGE_MAX_CHARS = parsePositiveInt(
	PUBLIC_USER_MESSAGE_MAX_CHARS,
	DEFAULT_USER_MESSAGE_MAX_CHARS
);
