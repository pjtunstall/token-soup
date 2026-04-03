import DOMPurify from 'isomorphic-dompurify';
import { marked } from 'marked';

/** Markdown → sanitized HTML for {@html ...}. */
export function renderMarkdown(src: string): string {
	const html = marked(src, { async: false });
	return DOMPurify.sanitize(html);
}

function escapeHtml(s: string): string {
	return s
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;');
}

function escapeRegExp(s: string): string {
	return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * Assistant bubbles: bold `name:` (from OpenRouter model id) like user display names; rest is inline markdown.
 */
export function renderAssistantBubble(fullText: string, assistantName: string): string {
	const label = assistantName.trim() || 'assistant';
	const re = new RegExp(`^${escapeRegExp(label)}:\\s*`, 'i');
	if (!re.test(fullText)) {
		return renderMarkdown(fullText);
	}
	const rest = fullText.replace(re, '');
	const body = marked.parseInline(rest, { async: false });
	const html = `<p><strong>${escapeHtml(label)}:</strong> ${body}</p>`;
	return DOMPurify.sanitize(html);
}
