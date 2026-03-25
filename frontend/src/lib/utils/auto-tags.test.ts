import { describe, it, expect } from 'vitest';
import { compileAutoTags, matchAutoTags } from './auto-tags';

describe('compileAutoTags', () => {
	it('normalizes mask to lowercase when ignore_case=true', () => {
		const [tag] = compileAutoTags([{ mask: 'Купить', label: 'покупки', ignore_case: true }]);
		expect(tag.mask).toBe('купить');
		expect(tag.ignoreCase).toBe(true);
	});

	it('preserves mask case when ignore_case=false', () => {
		const [tag] = compileAutoTags([{ mask: 'Купить', label: 'покупки', ignore_case: false }]);
		expect(tag.mask).toBe('Купить');
		expect(tag.ignoreCase).toBe(false);
	});

	it('returns empty array for empty input', () => {
		expect(compileAutoTags([])).toHaveLength(0);
	});
});

describe('matchAutoTags', () => {
	it('returns matching labels', () => {
		const compiled = compileAutoTags([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoTags('купить молоко', compiled)).toEqual(['покупки']);
	});

	it('returns empty array when no match', () => {
		const compiled = compileAutoTags([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoTags('позвонить другу', compiled)).toEqual([]);
	});

	it('is case-insensitive when configured', () => {
		const compiled = compileAutoTags([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoTags('КУПИТЬ хлеб', compiled)).toEqual(['покупки']);
	});

	it('is case-sensitive when configured', () => {
		const compiled = compileAutoTags([{ mask: 'купить', label: 'покупки', ignore_case: false }]);
		expect(matchAutoTags('КУПИТЬ хлеб', compiled)).toEqual([]);
		expect(matchAutoTags('купить хлеб', compiled)).toEqual(['покупки']);
	});

	it('returns multiple matching labels', () => {
		const compiled = compileAutoTags([
			{ mask: 'купить', label: 'покупки', ignore_case: true },
			{ mask: 'встреча', label: 'работа', ignore_case: true }
		]);
		expect(matchAutoTags('встреча и купить кофе', compiled)).toEqual(['покупки', 'работа']);
	});

	it('returns empty array for empty title', () => {
		const compiled = compileAutoTags([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoTags('', compiled)).toEqual([]);
	});
});
