import { PUBLIC_WS_PATH, PUBLIC_WS_URL } from '$env/static/public';

export function getWebSocketUrl(): string {
	if (PUBLIC_WS_URL) return PUBLIC_WS_URL;

	const path = PUBLIC_WS_PATH || '/ws';
	const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	return `${protocol}//${window.location.host}${path}`;
}
