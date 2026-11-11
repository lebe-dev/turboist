import { describe, expect, it } from 'vitest';
import { distinctProjectIds, todayProjectsStore } from './todayProjects.svelte';

describe('distinctProjectIds', () => {
	it('keeps first-appearance order and drops duplicates and inbox tasks', () => {
		const tasks = [{ projectId: 5 }, { projectId: null }, { projectId: 2 }, { projectId: 5 }];
		expect(distinctProjectIds(tasks)).toEqual([5, 2]);
	});
});

describe('todayProjectsStore', () => {
	it('holds the published ids until cleared', () => {
		expect(todayProjectsStore.known).toBe(false);
		todayProjectsStore.set([3, 4]);
		expect(todayProjectsStore.ids).toEqual([3, 4]);
		expect(todayProjectsStore.known).toBe(true);
		todayProjectsStore.clear();
		expect(todayProjectsStore.ids).toEqual([]);
		expect(todayProjectsStore.known).toBe(false);
	});

	it('knows an empty Today apart from an unpublished one', () => {
		todayProjectsStore.set([]);
		expect(todayProjectsStore.known).toBe(true);
		todayProjectsStore.clear();
	});
});
