import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ConfigResponse } from '../api/types';

const getConfig = vi.fn();
const getIfChanged = vi.fn();
vi.mock('../api/endpoints/config', () => ({
	config: {
		get: () => getConfig(),
		getIfChanged: () => getIfChanged()
	}
}));

// Partial mock: `getApiClient` throws without a real client, but NOT_MODIFIED
// must keep its identity — the store compares the sentinel by reference.
vi.mock('../api/client', async (importOriginal) => {
	const actual = await importOriginal<typeof import('../api/client')>();
	return { ...actual, getApiClient: () => ({}) };
});

import { NOT_MODIFIED } from '../api/client';
import { configStore } from './config.svelte';
import { contextsStore } from './contexts.svelte';
import { projectsStore } from './projects.svelte';
import { labelsStore } from './labels.svelte';
import { settingsStore } from './settings.svelte';
import { appSettingsStore } from './appSettings.svelte';
import { userStateStore } from './userState.svelte';
import { troikiStore } from './troiki.svelte';
import { planStatsStore } from './planStats.svelte';
import { inboxStatsStore } from './inboxStats.svelte';
import { pinnedTasksStore } from './pinnedTasks.svelte';
import { harpoonStore } from './harpoon.svelte';
import { templatesStore } from './templates.svelte';

function payload(over: Record<string, unknown> = {}): ConfigResponse {
	return {
		timezone: 'UTC',
		contexts: [{ id: 1, name: 'Work' }],
		projects: [{ id: 2, title: 'Proj', isPinned: false }],
		labels: [{ id: 3, name: 'lbl' }],
		settings: { locale: 'en' },
		appSettings: { autoLabels: [], projectSuggestions: [] },
		userState: { activeContextId: 1 },
		troiki: { started: true },
		planStats: { week: 4, backlog: 7 },
		inboxStats: { count: 2, warnThresholdExceeded: true },
		pinnedTasks: [{ id: 10, title: 'pinned' }],
		harpoon: { slots: [{ kind: 'task', id: 10, title: 'pinned' }] },
		taskTemplates: [{ id: 20, name: 'Onboard' }],
		...over
	} as unknown as ConfigResponse;
}

function clearAll(): void {
	configStore.clear();
	contextsStore.setItems([]);
	projectsStore.setItems([]);
	labelsStore.setItems([]);
	userStateStore.clear();
	planStatsStore.clear();
	inboxStatsStore.clear();
	pinnedTasksStore.setItems([]);
	harpoonStore.setSlots([]);
	templatesStore.clear();
}

describe('configStore.load (boot)', () => {
	beforeEach(() => {
		getConfig.mockReset();
		getIfChanged.mockReset();
		clearAll();
	});
	afterEach(clearAll);

	// One round-trip has to hydrate every shell store: nothing else fetches these
	// on boot any more, so a slice dropped here is a silently empty sidebar.
	it('fans every embedded slice out to its domain store', async () => {
		getConfig.mockResolvedValue(payload());

		await configStore.load();

		expect(contextsStore.items.map((c) => c.id)).toEqual([1]);
		expect(projectsStore.items.map((p) => p.id)).toEqual([2]);
		expect(labelsStore.items.map((l) => l.id)).toEqual([3]);
		expect(settingsStore.value.locale).toBe('en');
		expect(appSettingsStore.value.autoLabels).toEqual([]);
		expect(userStateStore.activeContextId).toBe(1);
		expect(troikiStore.value).toEqual({ started: true });
		expect(planStatsStore.value).toEqual({ week: 4, backlog: 7 });
		expect(inboxStatsStore.count).toBe(2);
		expect(inboxStatsStore.warnThresholdExceeded).toBe(true);
		expect(pinnedTasksStore.items.map((t) => t.id)).toEqual([10]);
		// Both used to be separate out-of-band GETs after boot.
		expect(harpoonStore.slots.map((s) => s.id)).toEqual([10]);
		expect(templatesStore.items.map((t) => t.id)).toEqual([20]);
		expect(templatesStore.loaded).toBe(true);
	});

	// An offline-first launch right after upgrading can serve a pre-v1.15 config
	// payload out of IndexedDB. The SCHEMA_VERSION bump is the primary defence;
	// this asserts the belt does not throw and blank the whole boot.
	it('tolerates a cached pre-v1.15 payload with no harpoon/taskTemplates', async () => {
		harpoonStore.setSlots([{ kind: 'task', id: 99, title: 'stale' }]);
		getConfig.mockResolvedValue(payload({ harpoon: undefined, taskTemplates: undefined }));

		await expect(configStore.load()).resolves.toBeTruthy();

		expect(harpoonStore.slots).toEqual([]);
		expect(templatesStore.items).toEqual([]);
	});
});

describe('configStore.refresh (mid-session resync)', () => {
	beforeEach(() => {
		getConfig.mockReset();
		getIfChanged.mockReset();
		clearAll();
	});
	afterEach(clearAll);

	// Without a config already in hand a 304 would leave the shell with nothing
	// to render, so the first pull must be unconditional.
	it('issues an unconditional GET when no config is held yet', async () => {
		getConfig.mockResolvedValue(payload());

		const res = await configStore.refresh();

		expect(getConfig).toHaveBeenCalledTimes(1);
		expect(getIfChanged).not.toHaveBeenCalled();
		expect(res).not.toBeNull();
		expect(contextsStore.items.map((c) => c.id)).toEqual([1]);
	});

	it('issues a conditional GET once a config is held', async () => {
		getConfig.mockResolvedValue(payload());
		await configStore.load();
		getIfChanged.mockResolvedValue(payload({ contexts: [{ id: 42, name: 'New' }] }));

		await configStore.refresh();

		expect(getIfChanged).toHaveBeenCalledTimes(1);
		expect(contextsStore.items.map((c) => c.id)).toEqual([42]);
	});

	// The common case on a phone: every unlock reconnects SSE and triggers a
	// catch-up refresh that finds nothing new. Nothing may be re-applied, or the
	// fan-out re-renders the whole shell for no reason.
	it('returns null and applies nothing on 304', async () => {
		getConfig.mockResolvedValue(payload());
		await configStore.load();
		const held = configStore.value;

		// A local optimistic change that a needless fan-out would revert.
		pinnedTasksStore.setItems([]);
		getIfChanged.mockResolvedValue(NOT_MODIFIED);

		expect(await configStore.refresh()).toBeNull();

		expect(pinnedTasksStore.items).toEqual([]);
		expect(configStore.value).toBe(held);
	});

	// settings/appSettings are written optimistically from the settings page and
	// have no SSE scope; re-applying the server copy would revert a toggle whose
	// PATCH is still in flight (and desync the i18n runtime for `locale`).
	it('does not re-apply settings or appSettings', async () => {
		getConfig.mockResolvedValue(payload());
		await configStore.load();

		settingsStore.setValue({ locale: 'ru' } as never);
		appSettingsStore.setValue({ autoLabels: [{ id: 1 }], projectSuggestions: [] } as never);
		getIfChanged.mockResolvedValue(payload());

		await configStore.refresh();

		expect(settingsStore.value.locale).toBe('ru');
		expect(appSettingsStore.value.autoLabels).toHaveLength(1);
	});

	it('still refreshes the server-owned slices, harpoon and templates included', async () => {
		getConfig.mockResolvedValue(payload());
		await configStore.load();
		getIfChanged.mockResolvedValue(
			payload({
				planStats: { week: 1, backlog: 0 },
				harpoon: { slots: [] },
				taskTemplates: [{ id: 21, name: 'Other' }]
			})
		);

		await configStore.refresh();

		expect(planStatsStore.value).toEqual({ week: 1, backlog: 0 });
		expect(harpoonStore.slots).toEqual([]);
		expect(templatesStore.items.map((t) => t.id)).toEqual([21]);
	});
});
