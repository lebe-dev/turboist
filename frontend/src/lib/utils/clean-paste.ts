import { cleanTrackingParams } from '$lib/utils';

/**
 * Svelte action: intercepts paste events and strips tracking parameters from URLs.
 */
export function cleanPaste(node: HTMLElement) {
	function handler(e: ClipboardEvent) {
		const text = e.clipboardData?.getData('text/plain');
		if (!text) return;

		const cleaned = cleanTrackingParams(text);
		if (cleaned === text) return;

		e.preventDefault();
		const el = node as HTMLTextAreaElement | HTMLInputElement;
		const start = el.selectionStart ?? 0;
		const end = el.selectionEnd ?? 0;
		const before = el.value.slice(0, start);
		const after = el.value.slice(end);
		el.value = before + cleaned + after;
		const cursor = start + cleaned.length;
		el.setSelectionRange(cursor, cursor);
		el.dispatchEvent(new Event('input', { bubbles: true }));
	}

	node.addEventListener('paste', handler as EventListener);
	return {
		destroy() {
			node.removeEventListener('paste', handler as EventListener);
		}
	};
}
