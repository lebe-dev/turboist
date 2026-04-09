import { logger } from '$lib/stores/logger';
import { createTroikiTask } from '$lib/api/client';
import type { TroikiSectionState, SectionClass, CreateTroikiTaskRequest } from '$lib/api/types';
import {
	wsClient,
	type SnapshotTroikiData,
	type DeltaTroikiData
} from '$lib/ws/client.svelte';

function createTroikiStore() {
	let active = $state(false);
	let sections = $state<TroikiSectionState[]>([]);
	let loading = $state(false);

	let cleanups: (() => void)[] = [];

	function handleSnapshot(data: unknown, _seq?: number): void {
		const d = data as SnapshotTroikiData;
		sections = d.sections;
		loading = false;
	}

	function handleDelta(data: unknown, _seq?: number): void {
		const d = data as DeltaTroikiData;
		sections = sections.map((existing) => {
			const updated = d.sections.find((s) => s.class === existing.class);
			return updated ?? existing;
		});
	}

	function enter(): void {
		active = true;
		loading = true;
		logger.log('troiki', 'entering troiki view');

		cleanups.push(wsClient.onMessage('snapshot', 'troiki', handleSnapshot));
		cleanups.push(wsClient.onMessage('delta', 'troiki', handleDelta));

		wsClient.subscribe('troiki', {});
	}

	function exit(): void {
		active = false;
		logger.log('troiki', 'exiting troiki view');

		for (const cleanup of cleanups) cleanup();
		cleanups = [];
		wsClient.unsubscribe('troiki');

		sections = [];
	}

	function refresh(): void {
		wsClient.subscribe('troiki', {});
	}

	async function addTask(
		sectionClass: SectionClass,
		content: string,
		description: string
	): Promise<void> {
		const req: CreateTroikiTaskRequest = {
			section_class: sectionClass,
			content,
			description
		};

		try {
			await createTroikiTask(req);
		} catch (err) {
			logger.error('troiki', `addTask failed: ${err}`);
			refresh();
			throw err;
		}
	}

	return {
		get active() {
			return active;
		},
		get sections() {
			return sections;
		},
		get loading() {
			return loading;
		},
		enter,
		exit,
		refresh,
		addTask
	};
}

export const troikiStore = createTroikiStore();
