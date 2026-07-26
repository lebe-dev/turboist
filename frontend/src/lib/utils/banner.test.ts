import { describe, expect, it } from 'vitest';
import { BANNER_DAY_PART_OPTIONS, isBannerDayPart, isBannerVisible } from './banner';

describe('isBannerVisible', () => {
	it('hides an unpublished banner', () => {
		expect(
			isBannerVisible({ published: false, text: 'hi', dayPart: '', activePart: 'morning' })
		).toBe(false);
	});

	it('hides a blank banner', () => {
		expect(
			isBannerVisible({ published: true, text: '   \n', dayPart: '', activePart: 'morning' })
		).toBe(false);
	});

	it('shows an all-day banner regardless of the active phase', () => {
		for (const activePart of ['morning', 'afternoon', 'evening', null] as const) {
			expect(isBannerVisible({ published: true, text: 'hi', dayPart: '', activePart })).toBe(true);
		}
	});

	it('shows a phase-scoped banner only while that phase is active', () => {
		expect(
			isBannerVisible({ published: true, text: 'hi', dayPart: 'afternoon', activePart: 'afternoon' })
		).toBe(true);
	});

	it('hides a phase-scoped banner before its phase becomes active', () => {
		expect(
			isBannerVisible({ published: true, text: 'hi', dayPart: 'afternoon', activePart: 'morning' })
		).toBe(false);
	});

	it('hides a phase-scoped banner after its phase has passed', () => {
		expect(
			isBannerVisible({ published: true, text: 'hi', dayPart: 'afternoon', activePart: 'evening' })
		).toBe(false);
	});

	it('hides a phase-scoped banner when no phase is active', () => {
		expect(
			isBannerVisible({ published: true, text: 'hi', dayPart: 'morning', activePart: null })
		).toBe(false);
	});
});

describe('isBannerDayPart', () => {
	it('accepts the configurable values', () => {
		for (const o of BANNER_DAY_PART_OPTIONS) expect(isBannerDayPart(o.value)).toBe(true);
	});

	it('rejects anything else', () => {
		expect(isBannerDayPart('none')).toBe(false);
		expect(isBannerDayPart('night')).toBe(false);
	});
});
