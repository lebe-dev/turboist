import type { BannerDayPart, DayPart } from '$lib/api/types';

// BANNER_DAY_PART_OPTIONS is the ordered choice list shown in settings.
// The empty string means "all day" — no day-part restriction at all.
export const BANNER_DAY_PART_OPTIONS: Array<{ value: BannerDayPart; labelKey: string }> = [
	{ value: '', labelKey: 'settings.banner.dayPart.allDay' },
	{ value: 'morning', labelKey: 'settings.banner.dayPart.morning' },
	{ value: 'afternoon', labelKey: 'settings.banner.dayPart.afternoon' },
	{ value: 'evening', labelKey: 'settings.banner.dayPart.evening' }
];

export function isBannerDayPart(value: string): value is BannerDayPart {
	return BANNER_DAY_PART_OPTIONS.some((o) => o.value === value);
}

// isBannerVisible decides whether the Today banner should render. A configured
// day part narrows the banner to that phase only: it stays hidden until the
// phase becomes active and disappears once the phase is over.
export function isBannerVisible(opts: {
	published: boolean;
	text: string;
	dayPart: BannerDayPart;
	activePart: DayPart | null;
}): boolean {
	if (!opts.published) return false;
	if (opts.text.trim() === '') return false;
	if (opts.dayPart === '') return true;
	return opts.activePart === opts.dayPart;
}
