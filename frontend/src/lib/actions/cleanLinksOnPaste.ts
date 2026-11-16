import { stripTrackingParams } from '$lib/utils/linkCleanup';

/**
 * Svelte action for a text field: pasted text has the tracking parameters of its
 * links removed before it lands in the field, so the person sees the clean link.
 * Pasted text without such parameters is left to the browser.
 */
export function cleanLinksOnPaste(node: HTMLTextAreaElement | HTMLInputElement) {
	function onPaste(rawEvent: Event) {
		const event = rawEvent as ClipboardEvent;
		const pasted = event.clipboardData?.getData('text/plain');
		if (!pasted) return;
		const cleaned = stripTrackingParams(pasted);
		if (cleaned === pasted) return;
		event.preventDefault();
		node.setRangeText(cleaned, node.selectionStart ?? 0, node.selectionEnd ?? 0, 'end');
		node.dispatchEvent(new Event('input', { bubbles: true }));
	}
	node.addEventListener('paste', onPaste);
	return {
		destroy() {
			node.removeEventListener('paste', onPaste);
		}
	};
}
