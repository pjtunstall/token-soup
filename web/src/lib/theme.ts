/** localStorage key; keep in sync with inline script in app.html */
export const THEME_STORAGE_KEY = 'token-soup-theme';

export type ThemePreference = 'light' | 'dark';

export function themePreferenceFromDOM(): ThemePreference {
	if (typeof document === 'undefined') return 'dark';
	return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
}

export function applyTheme(theme: ThemePreference): void {
	document.documentElement.classList.toggle('dark', theme === 'dark');
	try {
		localStorage.setItem(THEME_STORAGE_KEY, theme);
	} catch {
		/* private mode / quota */
	}
}

export function toggleTheme(): ThemePreference {
	const next: ThemePreference = themePreferenceFromDOM() === 'dark' ? 'light' : 'dark';
	applyTheme(next);
	return next;
}
