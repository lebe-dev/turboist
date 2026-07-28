import { fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { configStore } from '$lib/stores/config.svelte';
import { settingsStore } from '$lib/stores/settings.svelte';
import { nowStore } from '$lib/stores/now.svelte';
import type {
	Label,
	LabelStatsItem,
	LabelStatsPeriod,
	LabelStatsPeriodStats,
	LabelStatsResponse
} from '$lib/api/types';
import LabelsPage from './+page.svelte';

const statsMock = vi.fn<() => Promise<LabelStatsResponse>>();

vi.mock('$lib/api/client', () => ({
	getApiClient: () => ({ fetch: vi.fn() })
}));

vi.mock('$lib/api/endpoints/labels', () => ({
	labels: {
		stats: () => statsMock()
	}
}));

function period(applied: number, previousApplied = 0, completed = 0): LabelStatsPeriodStats {
	return { applied, previousApplied, completed };
}

function makeLabel(id: number, name: string, over: Partial<Label> = {}): Label {
	return {
		id,
		name,
		color: 'blue',
		isFavourite: false,
		isPrivate: false,
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z',
		...over
	};
}

function makeItem(
	label: Label,
	periods: Partial<Record<LabelStatsPeriod, LabelStatsPeriodStats>>,
	over: Partial<LabelStatsItem> = {}
): LabelStatsItem {
	return {
		label,
		totalTasks: 0,
		openTasks: 0,
		overdue: 0,
		projects: 0,
		lastUsedAt: null,
		periods: { week: period(0), month: period(0), quarter: period(0), ...periods },
		...over
	};
}

function makeResponse(items: LabelStatsItem[]): LabelStatsResponse {
	return {
		ranges: {
			week: { start: '2026-07-20T00:00:00.000Z', end: '2026-07-27T00:00:00.000Z', days: 7 },
			month: { start: '2026-06-27T00:00:00.000Z', end: '2026-07-27T00:00:00.000Z', days: 30 },
			quarter: { start: '2026-04-28T00:00:00.000Z', end: '2026-07-27T00:00:00.000Z', days: 90 }
		},
		items
	};
}

// Value shown above a headline counter's caption (e.g. "Times applied").
function counterValue(caption: string): string {
	return screen.getByText(caption).previousElementSibling?.textContent?.trim() ?? '';
}

// Row order as rendered: every label link in the ranking/idle lists, in DOM order.
function renderedLabelNames(): string[] {
	return screen
		.getAllByRole('link')
		.map((el) => el.textContent?.trim() ?? '')
		.filter(Boolean);
}

beforeEach(() => {
	configStore.value = null;
	settingsStore.setValue({ ...settingsStore.value, publicView: false });
	statsMock.mockReset();
});

afterEach(() => {
	settingsStore.clear();
	vi.clearAllMocks();
});

describe('Labels stats page', () => {
	it('ranks labels by applications in the selected period', async () => {
		statsMock.mockResolvedValue(
			makeResponse([
				makeItem(makeLabel(1, 'rare'), { week: period(1) }),
				makeItem(makeLabel(2, 'frequent'), { week: period(9) }),
				makeItem(makeLabel(3, 'medium'), { week: period(4) })
			])
		);
		render(LabelsPage);

		expect(await screen.findByText('frequent')).toBeTruthy();
		expect(renderedLabelNames()).toEqual(['frequent', 'medium', 'rare']);
	});

	it('re-ranks without refetching when the period changes', async () => {
		statsMock.mockResolvedValue(
			makeResponse([
				makeItem(makeLabel(1, 'spiky'), { week: period(5), month: period(5) }),
				makeItem(makeLabel(2, 'steady'), { week: period(2), month: period(40) })
			])
		);
		render(LabelsPage);
		expect(await screen.findByText('spiky')).toBeTruthy();
		expect(renderedLabelNames()).toEqual(['spiky', 'steady']);

		await fireEvent.click(screen.getByRole('radio', { name: 'Month' }));

		expect(renderedLabelNames()).toEqual(['steady', 'spiky']);
		// All three windows arrive in one payload — switching must not hit the API.
		expect(statsMock).toHaveBeenCalledTimes(1);
	});

	it('shows the headline counters for the selected period', async () => {
		statsMock.mockResolvedValue(
			makeResponse([
				makeItem(makeLabel(1, 'alpha'), { week: period(3, 0, 2), month: period(30) }, { overdue: 1 }),
				makeItem(makeLabel(2, 'beta'), { week: period(4, 0, 5) }, { overdue: 2 }),
				makeItem(makeLabel(3, 'idle'), {}, { overdue: 4 })
			])
		);
		render(LabelsPage);
		expect(await screen.findByText('alpha')).toBeTruthy();

		expect(counterValue('Times applied')).toBe('7');
		expect(counterValue('Tasks completed')).toBe('7');
		// Overdue is the current backlog across every label, idle ones included.
		expect(counterValue('Overdue tasks')).toBe('7');
		expect(counterValue('Labels in use')).toBe('2/3');
	});

	it('moves the counters with the selected period', async () => {
		statsMock.mockResolvedValue(
			makeResponse([
				makeItem(makeLabel(1, 'alpha'), { week: period(3, 0, 1), month: period(30, 0, 12) })
			])
		);
		render(LabelsPage);
		expect(await screen.findByText('alpha')).toBeTruthy();
		expect(counterValue('Times applied')).toBe('3');

		await fireEvent.click(screen.getByRole('radio', { name: 'Quarter' }));
		// Nothing in the quarter window: the label drops out of the ranking.
		expect(counterValue('Times applied')).toBe('0');
		expect(counterValue('Labels in use')).toBe('0/1');

		await fireEvent.click(screen.getByRole('radio', { name: 'Month' }));
		expect(counterValue('Times applied')).toBe('30');
		expect(counterValue('Tasks completed')).toBe('12');
	});

	it('separates labels with no activity in the period and dates their last use', async () => {
		configStore.value = { timezone: 'UTC' } as never;
		// The age caption is relative to `nowStore.now`, a module singleton that reads
		// the clock at import time — a pinned system time cannot move it afterwards.
		// Anchor the timestamp to that same clock instead of to a calendar date.
		const yesterday = new Date(nowStore.now.getTime() - 24 * 60 * 60 * 1000).toISOString();
		statsMock.mockResolvedValue(
			makeResponse([
				makeItem(makeLabel(1, 'busy'), { week: period(2) }),
				makeItem(makeLabel(2, 'yesterday-tag'), {}, { lastUsedAt: yesterday }),
				makeItem(makeLabel(3, 'never-tag'), {})
			])
		);
		render(LabelsPage);

		expect(await screen.findByText('busy')).toBeTruthy();
		expect(screen.getByRole('heading', { name: /Unused in this period \(2\)/ })).toBeTruthy();
		expect(screen.getByText('used yesterday')).toBeTruthy();
		expect(screen.getByText('never used')).toBeTruthy();
		// The idle ones come after the ranked one, most-recently-used first.
		expect(renderedLabelNames()).toEqual(['busy', 'yesterday-tag', 'never-tag']);
	});

	it('reports the trend against the previous window', async () => {
		statsMock.mockResolvedValue(
			makeResponse([
				makeItem(makeLabel(1, 'rising'), { week: period(6, 2) }),
				makeItem(makeLabel(2, 'falling'), { week: period(1, 5) })
			])
		);
		render(LabelsPage);

		expect(await screen.findByText('rising')).toBeTruthy();
		// +4 and -4 deltas, rendered as bare magnitudes next to a trend icon.
		expect(screen.getByTitle('2 in the previous period')).toBeTruthy();
		expect(screen.getByTitle('5 in the previous period')).toBeTruthy();
	});

	it('hides private labels in public view', async () => {
		settingsStore.setValue({ ...settingsStore.value, publicView: true });
		statsMock.mockResolvedValue(
			makeResponse([
				makeItem(makeLabel(1, 'public-tag'), { week: period(3) }),
				makeItem(makeLabel(2, 'private-tag', { isPrivate: true }), { week: period(9) })
			])
		);
		render(LabelsPage);

		expect(await screen.findByText('public-tag')).toBeTruthy();
		expect(screen.queryByText('private-tag')).toBeNull();
	});

	it('shows the empty state when there are no labels at all', async () => {
		statsMock.mockResolvedValue(makeResponse([]));
		render(LabelsPage);

		expect(await screen.findByText('No labels yet')).toBeTruthy();
	});

	it('reports when labels exist but none was used in the period', async () => {
		statsMock.mockResolvedValue(
			makeResponse([makeItem(makeLabel(1, 'dormant'), { quarter: period(4) })])
		);
		render(LabelsPage);

		expect(await screen.findByText('No label was used in this period.')).toBeTruthy();
		// It is still reachable — listed as a cleanup candidate, not hidden.
		expect(screen.getByText('dormant')).toBeTruthy();
	});

	it('links each label to its task list', async () => {
		statsMock.mockResolvedValue(
			makeResponse([makeItem(makeLabel(42, 'linked'), { week: period(1) })])
		);
		render(LabelsPage);

		const link = await screen.findByRole('link', { name: 'linked' });
		expect(link.getAttribute('href')).toBe('/label/42');
	});
});
