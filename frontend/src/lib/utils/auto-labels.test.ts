import { describe, it, expect } from 'vitest';
import { compileAutoLabels, matchAutoLabels } from './auto-labels';

describe('compileAutoLabels', () => {
	it('normalizes mask to lowercase when ignore_case=true', () => {
		const [tag] = compileAutoLabels([{ mask: 'Купить', label: 'покупки', ignore_case: true }]);
		expect(tag.mask).toBe('купить');
		expect(tag.ignoreCase).toBe(true);
	});

	it('preserves mask case when ignore_case=false', () => {
		const [tag] = compileAutoLabels([{ mask: 'Купить', label: 'покупки', ignore_case: false }]);
		expect(tag.mask).toBe('Купить');
		expect(tag.ignoreCase).toBe(false);
	});

	it('returns empty array for empty input', () => {
		expect(compileAutoLabels([])).toHaveLength(0);
	});
});

describe('matchAutoLabels', () => {
	it('returns matching labels', () => {
		const compiled = compileAutoLabels([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoLabels('купить молоко', compiled)).toEqual(['покупки']);
	});

	it('returns empty array when no match', () => {
		const compiled = compileAutoLabels([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoLabels('позвонить другу', compiled)).toEqual([]);
	});

	it('is case-insensitive when configured', () => {
		const compiled = compileAutoLabels([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoLabels('КУПИТЬ хлеб', compiled)).toEqual(['покупки']);
	});

	it('is case-sensitive when configured', () => {
		const compiled = compileAutoLabels([{ mask: 'купить', label: 'покупки', ignore_case: false }]);
		expect(matchAutoLabels('КУПИТЬ хлеб', compiled)).toEqual([]);
		expect(matchAutoLabels('купить хлеб', compiled)).toEqual(['покупки']);
	});

	it('returns multiple matching labels', () => {
		const compiled = compileAutoLabels([
			{ mask: 'купить', label: 'покупки', ignore_case: true },
			{ mask: 'встреча', label: 'работа', ignore_case: true }
		]);
		expect(matchAutoLabels('встреча и купить кофе', compiled)).toEqual(['покупки', 'работа']);
	});

	it('returns empty array for empty title', () => {
		const compiled = compileAutoLabels([{ mask: 'купить', label: 'покупки', ignore_case: true }]);
		expect(matchAutoLabels('', compiled)).toEqual([]);
	});
});
