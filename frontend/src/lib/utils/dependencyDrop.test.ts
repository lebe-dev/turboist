import { describe, expect, it } from 'vitest';
import {
	dependencyCandidates,
	dropModeForOffset,
	isNestNoop,
	nestRefusal,
	refusalFromServer,
	structuralRefusal
} from './dependencyDrop';
import type { Task, TaskStatus } from '../api/types';

function task(
	id: number,
	parentId: number | null = null,
	status: TaskStatus = 'open',
	title = `t${id}`
): Task {
	return {
		id,
		title,
		description: '',
		inboxId: null,
		contextId: null,
		projectId: null,
		sectionId: null,
		parentId,
		priority: 'no-priority',
		status,
		dueAt: null,
		dueHasTime: false,
		deadlineAt: null,
		deadlineHasTime: false,
		dayPart: 'none',
		planState: 'none',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		isComplex: false,
		completedAt: null,
		recurrenceRule: null,
		sourceTaskId: null,
		postponeCount: 0,
		autoSortedAt: null,
		autoSortUndecidedAt: null,
		blockedByCount: 0,
		relationCount: 0,
		labels: [],
		url: '',
		createdAt: '',
		updatedAt: ''
	};
}

// 1
// ├─ 2
// │  └─ 4
// └─ 3
// 5 (completed)
// 6
const tree = [task(1), task(2, 1), task(3, 1), task(4, 2), task(5, null, 'completed'), task(6)];

describe('structuralRefusal', () => {
	it('allows a sibling and an unrelated branch', () => {
		expect(structuralRefusal(tree, 2, 3)).toBeNull();
		expect(structuralRefusal(tree, 4, 6)).toBeNull();
		expect(structuralRefusal(tree, 4, 3)).toBeNull();
	});

	it('refuses an ancestor at any depth', () => {
		expect(structuralRefusal(tree, 4, 2)).toBe('ancestor');
		expect(structuralRefusal(tree, 4, 1)).toBe('ancestor');
	});

	it('refuses a descendant at any depth', () => {
		expect(structuralRefusal(tree, 1, 2)).toBe('descendant');
		expect(structuralRefusal(tree, 1, 4)).toBe('descendant');
	});

	it('refuses a task that is no longer open', () => {
		expect(structuralRefusal(tree, 6, 5)).toBe('completed');
	});

	it('does not treat the dragged row or an unknown row as a target', () => {
		expect(structuralRefusal(tree, 2, 2)).toBeNull();
		expect(structuralRefusal(tree, 2, 99)).toBeNull();
	});

	it('survives a parent cycle in malformed data', () => {
		const looped = [task(7, 8), task(8, 7), task(9)];
		expect(structuralRefusal(looped, 7, 9)).toBeNull();
	});
});

describe('dependencyCandidates', () => {
	it('lists every open row the tree allows, minus the dragged one', () => {
		expect(dependencyCandidates(tree, 4)).toEqual([3, 6]);
		expect(dependencyCandidates(tree, 1)).toEqual([6]);
	});

	it('skips rows still queued offline', () => {
		expect(dependencyCandidates([task(1), task(-3)], 1)).toEqual([]);
	});
});

describe('refusalFromServer', () => {
	it('shows only the refusals a visible row can meet', () => {
		expect(refusalFromServer('relation_exists')).toBe('exists');
		expect(refusalFromServer('relation_cycle')).toBe('cycle');
		expect(refusalFromServer('relation_self')).toBeNull();
		expect(refusalFromServer('not_found')).toBeNull();
	});
});

describe('dropModeForOffset', () => {
	it('reads the top and bottom quarter of a row as dependency', () => {
		expect(dropModeForOffset(0)).toBe('dependency');
		expect(dropModeForOffset(0.1)).toBe('dependency');
		expect(dropModeForOffset(0.9)).toBe('dependency');
		expect(dropModeForOffset(1)).toBe('dependency');
	});

	it('reads the middle half of a row as nest', () => {
		expect(dropModeForOffset(0.25)).toBe('nest');
		expect(dropModeForOffset(0.5)).toBe('nest');
		expect(dropModeForOffset(0.75)).toBe('nest');
	});
});

describe('isNestNoop', () => {
	it('is a no-op only when the target is already the dragged row\'s parent', () => {
		expect(isNestNoop(tree, 2, 1)).toBe(true);
		expect(isNestNoop(tree, 4, 2)).toBe(true);
		expect(isNestNoop(tree, 4, 1)).toBe(false);
		expect(isNestNoop(tree, 2, 3)).toBe(false);
	});
});

describe('nestRefusal', () => {
	it('allows nesting under a sibling, an unrelated branch, or an ancestor', () => {
		expect(nestRefusal(tree, 2, 3)).toBeNull();
		expect(nestRefusal(tree, 4, 6)).toBeNull();
		// Moving 4 up to sit directly under its grandparent is a legitimate reparent,
		// not a cycle — only nesting under one's own descendant is refused.
		expect(nestRefusal(tree, 4, 1)).toBeNull();
	});

	it('refuses nesting a task under its own descendant', () => {
		expect(nestRefusal(tree, 1, 2)).toBe('descendant');
		expect(nestRefusal(tree, 1, 4)).toBe('descendant');
		expect(nestRefusal(tree, 2, 4)).toBe('descendant');
	});

	it('refuses nesting under a task that is no longer open', () => {
		expect(nestRefusal(tree, 6, 5)).toBe('completed');
	});

	it('does not treat an unknown target as refused', () => {
		expect(nestRefusal(tree, 2, 99)).toBeNull();
	});
});
