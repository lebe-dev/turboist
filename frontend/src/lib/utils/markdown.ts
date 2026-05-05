export type MarkdownSegment =
	| { type: 'text'; value: string }
	| { type: 'link'; text: string; href: string };

const LINK_RE = /\[([^\]]+)\]\(([^)\s]+)\)/g;
const SAFE_SCHEME = /^(https?:\/\/|mailto:|\/|#)/i;

export function parseMarkdownLinks(input: string): MarkdownSegment[] {
	if (!input) return [];
	const segments: MarkdownSegment[] = [];
	let lastIndex = 0;
	LINK_RE.lastIndex = 0;
	let match: RegExpExecArray | null;
	while ((match = LINK_RE.exec(input)) !== null) {
		const [whole, text, href] = match;
		if (!SAFE_SCHEME.test(href)) continue;
		if (match.index > lastIndex) {
			segments.push({ type: 'text', value: input.slice(lastIndex, match.index) });
		}
		segments.push({ type: 'link', text, href });
		lastIndex = match.index + whole.length;
	}
	if (lastIndex < input.length) {
		segments.push({ type: 'text', value: input.slice(lastIndex) });
	}
	return segments;
}

export function hasMarkdownLink(input: string): boolean {
	if (!input) return false;
	LINK_RE.lastIndex = 0;
	const m = LINK_RE.exec(input);
	if (!m) return false;
	return SAFE_SCHEME.test(m[2]);
}
