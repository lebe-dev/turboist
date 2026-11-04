// Turns what another app shares with Turboist (Web Share Target on the PWA,
// ACTION_SEND on Android) into a QuickAdd prefill.
//
// Apps disagree on where the link goes: Chrome puts it in `url`, most Android
// apps put "Page title https://…" into `text`. A link found anywhere wins the
// title on its own, so the server's link-title job can name the task; the
// remaining words land in the description instead of being lost.

export interface SharedContent {
	title?: string | null;
	text?: string | null;
	url?: string | null;
}

export interface SharedPrefill {
	title: string;
	description: string;
}

const LINK_RE = /https?:\/\/[^\s<>"']+/i;
// Trailing punctuation that is almost always prose, not part of the link.
const TRAILING_PUNCT_RE = /[.,;:!?)\]}»”'"]+$/;

function clean(v: string | null | undefined): string {
	return (v ?? '').trim();
}

function findLink(s: string): string | null {
	const m = LINK_RE.exec(s);
	return m ? m[0].replace(TRAILING_PUNCT_RE, '') : null;
}

export function prefillFromShare(content: SharedContent): SharedPrefill | null {
	const title = clean(content.title);
	const text = clean(content.text);
	const url = clean(content.url);
	const link = findLink(url) ?? findLink(text) ?? findLink(title);

	if (!link) {
		const first = title || text;
		if (!first) return null;
		const rest = title && text && text !== title ? text : '';
		return { title: first, description: rest };
	}

	// Words around the link (often the page title the sharing app added) go to
	// the description so nothing is dropped.
	const leftovers = [title, text.replace(link, '').trim()]
		.map((s) =>
			s
				.replace(link, '')
				.replace(/\s+[.,;:!?]+$/, '')
				.trim()
		)
		.filter((s, i, all) => s !== '' && all.indexOf(s) === i);
	return { title: link, description: leftovers.join('\n\n') };
}
