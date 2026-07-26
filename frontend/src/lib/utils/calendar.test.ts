import { beforeEach, describe, expect, it } from 'vitest';
import type { CalendarEvent } from '$lib/api/types';
import { ApiError } from '$lib/api/errors';
import { calendarReauthStore } from '$lib/stores/calendarReauth.svelte';
import { calendarEventsOrEmpty, isPastCalendarEvent } from './calendar';

function makeEvent(overrides: Partial<CalendarEvent>): CalendarEvent {
	return {
		id: 'event-1',
		sourceId: 1,
		sourceName: 'Calendar',
		sourceColor: '#3b82f6',
		provider: 'google',
		externalId: 'external-1',
		title: 'Event',
		location: '',
		start: '2026-05-16T10:00:00.000Z',
		end: '2026-05-16T11:00:00.000Z',
		allDay: false,
		htmlLink: '',
		...overrides
	};
}

describe('isPastCalendarEvent', () => {
	it('treats timed events as past after their end time', () => {
		const event = makeEvent({ end: '2026-05-16T11:00:00.000Z' });
		expect(isPastCalendarEvent(event, new Date('2026-05-16T11:00:00.000Z'), 'UTC')).toBe(true);
		expect(isPastCalendarEvent(event, new Date('2026-05-16T10:59:59.000Z'), 'UTC')).toBe(false);
	});

	it('keeps all-day events visible until their exclusive end date', () => {
		const event = makeEvent({
			allDay: true,
			startDate: '2026-05-16',
			endDate: '2026-05-17'
		});
		expect(isPastCalendarEvent(event, new Date('2026-05-16T12:00:00.000Z'), 'UTC')).toBe(false);
		expect(isPastCalendarEvent(event, new Date('2026-05-17T00:00:00.000Z'), 'UTC')).toBe(true);
	});
});

describe('calendarEventsOrEmpty', () => {
	beforeEach(() => calendarReauthStore.clear());

	it('flags reauth when the fetch fails with calendar_reauth_required', async () => {
		const load = Promise.reject(
			new ApiError('calendar_reauth_required', 'authorization expired', 409)
		);
		await expect(calendarEventsOrEmpty(load)).resolves.toEqual([]);
		expect(calendarReauthStore.needed).toBe(true);
	});

	it('clears the reauth flag after a successful fetch', async () => {
		calendarReauthStore.flag();
		await expect(calendarEventsOrEmpty(Promise.resolve([]))).resolves.toEqual([]);
		expect(calendarReauthStore.needed).toBe(false);
	});

	it('leaves the reauth flag untouched on an unrelated failure', async () => {
		calendarReauthStore.flag();
		const load = Promise.reject(new ApiError('internal_error', 'boom', 500));
		await expect(calendarEventsOrEmpty(load)).resolves.toEqual([]);
		expect(calendarReauthStore.needed).toBe(true);
	});
});
