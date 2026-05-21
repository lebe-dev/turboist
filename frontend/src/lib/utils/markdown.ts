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

const RICH_MARKDOWN_RE = /(^|\n)#{1,6}\s+\S|\*\*[^\s*][^*]*\*\*|(^|[\s])\*[^\s*][^*]*\*|(^|[\s])_[^\s_][^_]*_|`[^`]+`|(^|\n)([-*+]|\d+\.)\s+\S/;

export function hasMarkdownContent(input: string): boolean {
	if (!input) return false;
	if (hasMarkdownLink(input)) return true;
	return RICH_MARKDOWN_RE.test(input);
}

export type InlineSegment =
	| { type: 'text'; value: string }
	| { type: 'bold'; segments: InlineSegment[] }
	| { type: 'italic'; segments: InlineSegment[] }
	| { type: 'code'; value: string }
	| { type: 'link'; text: string; href: string };

export type MarkdownBlock =
	| { type: 'heading'; level: 1 | 2 | 3 | 4 | 5 | 6; segments: InlineSegment[] }
	| { type: 'paragraph'; segments: InlineSegment[] }
	| { type: 'list'; ordered: boolean; items: InlineSegment[][] };

const HEADING_RE = /^(#{1,6})\s+(.*)$/;
const UNORDERED_LIST_RE = /^[-*+]\s+(.*)$/;
const ORDERED_LIST_RE = /^\d+\.\s+(.*)$/;

export function parseInlineMarkdown(input: string): InlineSegment[] {
	if (!input) return [];
	const segments: InlineSegment[] = [];
	let i = 0;
	let textBuf = '';
	const flushText = () => {
		if (textBuf) {
			segments.push({ type: 'text', value: textBuf });
			textBuf = '';
		}
	};
	while (i < input.length) {
		const ch = input[i];
		if (ch === '`') {
			const end = input.indexOf('`', i + 1);
			if (end > i + 1) {
				flushText();
				segments.push({ type: 'code', value: input.slice(i + 1, end) });
				i = end + 1;
				continue;
			}
		}
		if (ch === '[') {
			LINK_RE.lastIndex = i;
			const m = LINK_RE.exec(input);
			if (m && m.index === i && SAFE_SCHEME.test(m[2])) {
				flushText();
				segments.push({ type: 'link', text: m[1], href: m[2] });
				i += m[0].length;
				continue;
			}
		}
		if (ch === '*' || ch === '_') {
			const isDouble = input[i + 1] === ch;
			const marker = isDouble ? ch + ch : ch;
			const start = i + marker.length;
			if (start < input.length && input[start] !== ' ') {
				const end = input.indexOf(marker, start);
				if (end > start) {
					const closingChar = input[end - 1];
					if (closingChar !== ' ' && (isDouble || input[end + 1] !== ch)) {
						flushText();
						const inner = parseInlineMarkdown(input.slice(start, end));
						segments.push({ type: isDouble ? 'bold' : 'italic', segments: inner });
						i = end + marker.length;
						continue;
					}
				}
			}
		}
		textBuf += ch;
		i++;
	}
	flushText();
	return segments;
}

export function parseMarkdownBlocks(input: string): MarkdownBlock[] {
	if (!input) return [];
	const lines = input.replace(/\r\n?/g, '\n').split('\n');
	const blocks: MarkdownBlock[] = [];
	let paragraphLines: string[] = [];
	let listItems: string[] | null = null;
	let listOrdered = false;

	const flushParagraph = () => {
		if (paragraphLines.length === 0) return;
		const text = paragraphLines.join('\n');
		blocks.push({ type: 'paragraph', segments: parseInlineMarkdown(text) });
		paragraphLines = [];
	};
	const flushList = () => {
		if (!listItems) return;
		blocks.push({
			type: 'list',
			ordered: listOrdered,
			items: listItems.map((item) => parseInlineMarkdown(item))
		});
		listItems = null;
	};

	for (const raw of lines) {
		const line = raw.trimEnd();
		if (line.trim() === '') {
			flushParagraph();
			flushList();
			continue;
		}
		const headingMatch = HEADING_RE.exec(line);
		if (headingMatch) {
			flushParagraph();
			flushList();
			const level = headingMatch[1].length as 1 | 2 | 3 | 4 | 5 | 6;
			blocks.push({ type: 'heading', level, segments: parseInlineMarkdown(headingMatch[2]) });
			continue;
		}
		const unorderedMatch = UNORDERED_LIST_RE.exec(line);
		const orderedMatch = !unorderedMatch ? ORDERED_LIST_RE.exec(line) : null;
		if (unorderedMatch || orderedMatch) {
			flushParagraph();
			const ordered = !!orderedMatch;
			if (listItems && listOrdered !== ordered) flushList();
			if (!listItems) {
				listItems = [];
				listOrdered = ordered;
			}
			listItems.push((unorderedMatch ?? orderedMatch)![1]);
			continue;
		}
		flushList();
		paragraphLines.push(line);
	}
	flushParagraph();
	flushList();
	return blocks;
}

export function stripMarkdownSyntax(input: string): string {
	if (!input) return '';
	const blocks = parseMarkdownBlocks(input);
	return blocks
		.map((block) => {
			if (block.type === 'list') {
				return block.items.map((item) => inlineToPlain(item)).join('\n');
			}
			return inlineToPlain(block.segments);
		})
		.join('\n')
		.replace(/\n{2,}/g, '\n')
		.trim();
}

function inlineToPlain(segments: InlineSegment[]): string {
	let out = '';
	for (const seg of segments) {
		if (seg.type === 'text') out += seg.value;
		else if (seg.type === 'code') out += seg.value;
		else if (seg.type === 'link') out += seg.text;
		else out += inlineToPlain(seg.segments);
	}
	return out;
}
