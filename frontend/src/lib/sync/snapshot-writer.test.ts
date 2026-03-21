import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Task, Meta } from '$lib/api/types';

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

const mockSaveTaskSnapshot = vi.fn(() => Promise.resolve());

vi.mock('$lib/sync/db', () => ({
	saveTaskSnapshot: mockSaveTaskSnapshot
}));

const tasks: Task[] = [
	{
		id: '1',
		content: 'Task 1',
		description: '',
		project_id: 'p1',
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 1,
		due: null,
		sub_task_count: 0,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2024-01-01',
		is_project_task: false,
		children: []
	}
];

const meta: Meta = {
	context: 'default',
	weekly_limit: 10,
	weekly_count: 0,
	backlog_limit: 20,
	backlog_count: 0
};

async function freshModule() {
	vi.resetModules();
	return import('./snapshot-writer');
}

describe('snapshot-writer', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		vi.clearAllMocks();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('scheduleSnapshotWrite writes after 2s debounce', async () => {
		const { scheduleSnapshotWrite } = await freshModule();

		scheduleSnapshotWrite('today', undefined, tasks, meta);

		expect(mockSaveTaskSnapshot).not.toHaveBeenCalled();

		await vi.advanceTimersByTimeAsync(2000);

		expect(mockSaveTaskSnapshot).toHaveBeenCalledWith('today', undefined, tasks, meta);
	});

	it('multiple calls within 2s only write last data once', async () => {
		const { scheduleSnapshotWrite } = await freshModule();

		const tasks2 = [{ ...tasks[0], content: 'Updated' }];

		scheduleSnapshotWrite('today', undefined, tasks, meta);
		scheduleSnapshotWrite('today', undefined, tasks2, meta);

		await vi.advanceTimersByTimeAsync(2000);

		expect(mockSaveTaskSnapshot).toHaveBeenCalledTimes(1);
		expect(mockSaveTaskSnapshot).toHaveBeenCalledWith('today', undefined, tasks2, meta);
	});

	it('writeSnapshotImmediate writes immediately and cancels pending', async () => {
		const { scheduleSnapshotWrite, writeSnapshotImmediate } = await freshModule();

		scheduleSnapshotWrite('today', undefined, tasks, meta);

		const immediateTasks = [{ ...tasks[0], content: 'Immediate' }];
		writeSnapshotImmediate('today', 'ctx-1', immediateTasks, meta);

		expect(mockSaveTaskSnapshot).toHaveBeenCalledTimes(1);
		expect(mockSaveTaskSnapshot).toHaveBeenCalledWith('today', 'ctx-1', immediateTasks, meta);

		// Pending scheduled write should be cancelled
		await vi.advanceTimersByTimeAsync(2000);
		expect(mockSaveTaskSnapshot).toHaveBeenCalledTimes(1);
	});

	it('visibilitychange flushes pending write', async () => {
		const { scheduleSnapshotWrite } = await freshModule();

		scheduleSnapshotWrite('today', undefined, tasks, meta);

		Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
		document.dispatchEvent(new Event('visibilitychange'));

		expect(mockSaveTaskSnapshot).toHaveBeenCalledWith('today', undefined, tasks, meta);

		Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
	});
});
