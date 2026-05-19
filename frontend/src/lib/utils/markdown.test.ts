import { describe, expect, it } from 'vitest';
import {
	hasMarkdownContent,
	parseInlineMarkdown,
	parseMarkdownBlocks,
	stripMarkdownSyntax,
	type InlineSegment
} from './markdown';

describe('parseInlineMarkdown', () => {
	it('returns empty for empty input', () => {
		expect(parseInlineMarkdown('')).toEqual([]);
	});

	it('returns plain text as text segment', () => {
		expect(parseInlineMarkdown('hello world')).toEqual([{ type: 'text', value: 'hello world' }]);
	});

	it('parses bold', () => {
		const segs = parseInlineMarkdown('a **bold** b');
		expect(segs).toEqual([
			{ type: 'text', value: 'a ' },
			{ type: 'bold', segments: [{ type: 'text', value: 'bold' }] },
			{ type: 'text', value: ' b' }
		]);
	});

	it('parses italic with *', () => {
		const segs = parseInlineMarkdown('a *it* b');
		expect(segs).toEqual([
			{ type: 'text', value: 'a ' },
			{ type: 'italic', segments: [{ type: 'text', value: 'it' }] },
			{ type: 'text', value: ' b' }
		]);
	});

	it('parses italic with _', () => {
		const segs = parseInlineMarkdown('a _it_ b');
		expect(segs).toEqual([
			{ type: 'text', value: 'a ' },
			{ type: 'italic', segments: [{ type: 'text', value: 'it' }] },
			{ type: 'text', value: ' b' }
		]);
	});

	it('parses inline code', () => {
		const segs = parseInlineMarkdown('use `console.log` here');
		expect(segs).toEqual([
			{ type: 'text', value: 'use ' },
			{ type: 'code', value: 'console.log' },
			{ type: 'text', value: ' here' }
		]);
	});

	it('parses link', () => {
		const segs = parseInlineMarkdown('see [docs](https://example.com)');
		expect(segs).toEqual([
			{ type: 'text', value: 'see ' },
			{ type: 'link', text: 'docs', href: 'https://example.com' }
		]);
	});

	it('rejects unsafe link scheme', () => {
		const segs = parseInlineMarkdown('see [x](javascript:alert(1))');
		expect(segs.some((s) => s.type === 'link')).toBe(false);
	});

	it('handles bold inside text without trailing space', () => {
		const segs = parseInlineMarkdown('**bold**!');
		expect(segs).toEqual([
			{ type: 'bold', segments: [{ type: 'text', value: 'bold' }] },
			{ type: 'text', value: '!' }
		]);
	});

	it('does not eat lone asterisk', () => {
		expect(parseInlineMarkdown('a * b')).toEqual([{ type: 'text', value: 'a * b' }]);
	});

	it('nests italic inside bold', () => {
		const segs = parseInlineMarkdown('**bold _it_ end**');
		const bold = segs[0] as Extract<InlineSegment, { type: 'bold' }>;
		expect(bold.type).toBe('bold');
		expect(bold.segments).toEqual([
			{ type: 'text', value: 'bold ' },
			{ type: 'italic', segments: [{ type: 'text', value: 'it' }] },
			{ type: 'text', value: ' end' }
		]);
	});
});

describe('parseMarkdownBlocks', () => {
	it('returns empty for empty input', () => {
		expect(parseMarkdownBlocks('')).toEqual([]);
	});

	it('parses a single paragraph', () => {
		const blocks = parseMarkdownBlocks('hello world');
		expect(blocks).toEqual([
			{ type: 'paragraph', segments: [{ type: 'text', value: 'hello world' }] }
		]);
	});

	it('parses heading levels', () => {
		const blocks = parseMarkdownBlocks('### My Title');
		expect(blocks).toEqual([
			{ type: 'heading', level: 3, segments: [{ type: 'text', value: 'My Title' }] }
		]);
	});

	it('splits paragraphs by blank lines', () => {
		const blocks = parseMarkdownBlocks('one\n\ntwo');
		expect(blocks).toHaveLength(2);
		expect(blocks[0].type).toBe('paragraph');
		expect(blocks[1].type).toBe('paragraph');
	});

	it('parses an unordered list', () => {
		const blocks = parseMarkdownBlocks('- a\n- b');
		expect(blocks).toEqual([
			{
				type: 'list',
				ordered: false,
				items: [[{ type: 'text', value: 'a' }], [{ type: 'text', value: 'b' }]]
			}
		]);
	});

	it('parses an ordered list', () => {
		const blocks = parseMarkdownBlocks('1. a\n2. b');
		expect(blocks).toEqual([
			{
				type: 'list',
				ordered: true,
				items: [[{ type: 'text', value: 'a' }], [{ type: 'text', value: 'b' }]]
			}
		]);
	});

	it('mixes heading and paragraph', () => {
		const blocks = parseMarkdownBlocks('## Section\nbody');
		expect(blocks).toHaveLength(2);
		expect(blocks[0]).toMatchObject({ type: 'heading', level: 2 });
		expect(blocks[1]).toMatchObject({ type: 'paragraph' });
	});
});

describe('hasMarkdownContent', () => {
	it('returns false for empty input', () => {
		expect(hasMarkdownContent('')).toBe(false);
	});

	it('returns false for plain text', () => {
		expect(hasMarkdownContent('just some text')).toBe(false);
	});

	it('detects headings', () => {
		expect(hasMarkdownContent('### Title')).toBe(true);
	});

	it('detects bold', () => {
		expect(hasMarkdownContent('a **bold** b')).toBe(true);
	});

	it('detects italic', () => {
		expect(hasMarkdownContent('a *it* b')).toBe(true);
	});

	it('detects code', () => {
		expect(hasMarkdownContent('use `x` here')).toBe(true);
	});

	it('detects lists', () => {
		expect(hasMarkdownContent('- one\n- two')).toBe(true);
	});

	it('detects links', () => {
		expect(hasMarkdownContent('[docs](https://x)')).toBe(true);
	});
});

describe('stripMarkdownSyntax', () => {
	it('strips bold and italic markers', () => {
		expect(stripMarkdownSyntax('**a** and *b*')).toBe('a and b');
	});

	it('strips heading markers', () => {
		expect(stripMarkdownSyntax('### Title')).toBe('Title');
	});

	it('strips list markers', () => {
		expect(stripMarkdownSyntax('- one\n- two')).toBe('one\ntwo');
	});

	it('keeps link text', () => {
		expect(stripMarkdownSyntax('see [docs](https://x.y)')).toBe('see docs');
	});

	it('returns empty for empty input', () => {
		expect(stripMarkdownSyntax('')).toBe('');
	});
});
