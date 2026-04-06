import DOMPurify from 'isomorphic-dompurify';
import { marked } from 'marked';

/** Markdown → sanitized HTML for {@html ...}. */
export function renderMarkdown(src: string): string {
	const html = marked(src, { async: false });
	return DOMPurify.sanitize(html);
}

function escapeRegExp(s: string): string {
	return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/** Split `ModelName: …rest` when the label matches (case-insensitive). */
export function splitAssistantLabelPrefix(
	fullText: string,
	assistantName: string
): { label: string; rest: string } | null {
	const label = assistantName.trim() || 'assistant';
	const re = new RegExp(`^${escapeRegExp(label)}:\\s*`, 'i');
	if (!re.test(fullText)) {
		return null;
	}
	const rest = fullText.replace(re, '');
	return { label, rest };
}

