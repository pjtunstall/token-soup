import { env } from '$env/dynamic/public';

const DEFAULT_ROLE_NAME_MAX_CHARS = 64;
const DEFAULT_ROLE_DESCRIPTION_MAX_CHARS = 2000;
const DEFAULT_USER_MESSAGE_MAX_CHARS = 12000;

function parsePositiveInt(v: string | undefined, fallback: number): number {
	if (!v) return fallback;
	const n = Number.parseInt(v, 10);
	if (!Number.isFinite(n) || n < 1) return fallback;
	return n;
}

export const ROLE_NAME_MAX_CHARS = parsePositiveInt(
	env.PUBLIC_ROLE_NAME_MAX_CHARS,
	DEFAULT_ROLE_NAME_MAX_CHARS
);
export const ROLE_DESCRIPTION_MAX_CHARS = parsePositiveInt(
	env.PUBLIC_ROLE_DESCRIPTION_MAX_CHARS,
	DEFAULT_ROLE_DESCRIPTION_MAX_CHARS
);
export const USER_MESSAGE_MAX_CHARS = parsePositiveInt(
	env.PUBLIC_USER_MESSAGE_MAX_CHARS,
	DEFAULT_USER_MESSAGE_MAX_CHARS
);
