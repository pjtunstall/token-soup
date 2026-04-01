import DOMPurify from 'isomorphic-dompurify';
import { marked } from 'marked';

/** Markdown → sanitized HTML for {@html ...}. */
export function renderMarkdown(src: string): string {
	const html = marked(src, { async: false });
	return DOMPurify.sanitize(html);
}
