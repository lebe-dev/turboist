import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import type { Task } from '$lib/api/types';

vi.mock('$lib/stores/logger', () => ({
	logger: { log: vi.fn(), warn: vi.fn(), error: vi.fn() }
}));

vi.mock('$lib/api/client', () => ({
	getTasks: vi.fn(() =>
		Promise.resolve({ tasks: [] })
	)
}));

vi.mock('$lib/ws/client.svelte', () => ({
	wsClient: {
		onStateChange: vi.fn(() => vi.fn())
	}
}));

function makeTask(id: string, projectId: string = 'p1'): Task {
	return {
		id,
		content: `Task ${id}`,
		description: '',
		project_id: projectId,
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 1,
		due: null,
		sub_task_count: 0,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2024-01-01T00:00:00Z',
		is_project_task: false,
		postpone_count: 0,
		children: []
	};
}

describe('projectTasksStore.removeTaskLocal', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	afterEach(() => {
		vi.resetModules();
	});

	async function setupWithTasks(tasks: Task[]) {
		const { getTasks } = await import('$lib/api/client');
		vi.mocked(getTasks).mockResolvedValueOnce({ tasks } as any);

		const { projectTasksStore } = await import('./project-tasks.svelte');
		await projectTasksStore.start();
		return { projectTasksStore };
	}

	it('removes a task from getProjectTasks result', async () => {
		const { projectTasksStore } = await setupWithTasks([
			makeTask('1', 'p1'),
			makeTask('2', 'p1'),
			makeTask('3', 'p1')
		]);

		expect(projectTasksStore.getProjectTasks('p1')).toHaveLength(3);

		projectTasksStore.removeTaskLocal('2');

		const result = projectTasksStore.getProjectTasks('p1');
		expect(result).toHaveLength(2);
		expect(result.map((t) => t.id)).toEqual(['1', '3']);
	});

	it('no-op when task ID does not exist', async () => {
		const { projectTasksStore } = await setupWithTasks([
			makeTask('1', 'p1'),
			makeTask('2', 'p1')
		]);

		projectTasksStore.removeTaskLocal('nonexistent');

		expect(projectTasksStore.getProjectTasks('p1')).toHaveLength(2);
	});

	it('removes task only from its project', async () => {
		const { projectTasksStore } = await setupWithTasks([
			makeTask('1', 'p1'),
			makeTask('2', 'p2')
		]);

		projectTasksStore.removeTaskLocal('1');

		expect(projectTasksStore.getProjectTasks('p1')).toHaveLength(0);
		expect(projectTasksStore.getProjectTasks('p2')).toHaveLength(1);
	});
});
