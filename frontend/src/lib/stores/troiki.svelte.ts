import { logger } from '$lib/stores/logger';
import { createTroikiTask, getTroikiCompleted } from '$lib/api/client';
import type { Task, TroikiCompletedSection, TroikiSectionState, SectionClass, CreateTroikiTaskRequest } from '$lib/api/types';
import {
	wsClient,
	type SnapshotTroikiData,
	type DeltaTroikiData
} from '$lib/ws/client.svelte';

function buildSectionTree(tasks: Task[]): Task[] {
	const byId = new Map<string, Task>();
	for (const t of tasks) {
		byId.set(t.id, { ...t, children: [] });
	}
	const roots: Task[] = [];
	for (const t of tasks) {
		const node = byId.get(t.id)!;
		if (t.parent_id && byId.has(t.parent_id)) {
			byId.get(t.parent_id)!.children.push(node);
		} else {
			roots.push(node);
		}
	}
	return roots;
}

function processSection(section: TroikiSectionState): TroikiSectionState {
	return { ...section, tasks: buildSectionTree(section.tasks) };
}

function createTroikiStore() {
	let active = $state(false);
	let sections = $state<TroikiSectionState[]>([]);
	let loading = $state(false);
	let completedSections = $state<TroikiCompletedSection[]>([]);

	let cleanups: (() => void)[] = [];

	function handleSnapshot(data: unknown, _seq?: number): void {
		const d = data as SnapshotTroikiData;
		sections = d.sections.map(processSection);
		loading = false;
	}

	function handleDelta(data: unknown, _seq?: number): void {
		const d = data as DeltaTroikiData;
		sections = sections.map((existing) => {
			const updated = d.sections.find((s) => s.class === existing.class);
			return updated ? processSection(updated) : existing;
		});
	}

	function enter(): void {
		active = true;
		loading = true;
		logger.log('troiki', 'entering troiki view');

		cleanups.push(wsClient.onMessage('snapshot', 'troiki', handleSnapshot));
		cleanups.push(wsClient.onMessage('delta', 'troiki', handleDelta));

		wsClient.subscribe('troiki', {});

		getTroikiCompleted()
			.then((data) => { completedSections = data.sections; })
			.catch((err) => { logger.error('troiki', `fetch completed failed: ${err}`); });
	}

	function exit(): void {
		active = false;
		logger.log('troiki', 'exiting troiki view');

		for (const cleanup of cleanups) cleanup();
		cleanups = [];
		wsClient.unsubscribe('troiki');

		sections = [];
		completedSections = [];
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
		get completedSections() {
			return completedSections;
		},
		enter,
		exit,
		refresh,
		addTask
	};
}

export const troikiStore = createTroikiStore();
