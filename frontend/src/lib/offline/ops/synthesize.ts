import type { Task, TaskInput, TaskStatus } from '../../api/types';
import type { NowFn } from './types';

// Pure Task builders shared by the ops' `synthesizeResponse` (§4.5). Leaf module:
// depends only on the DTO types, so it never enters the ops/registry cycle.

/** Default wall clock for synthesized timestamps (ISO-8601 UTC). */
export const defaultNow: NowFn = () => new Date().toISOString();

/** Read an optional `completedAt` string off a mutation body. */
export function readCompletedAt(body: unknown): string | undefined {
	if (body && typeof body === 'object' && 'completedAt' in body) {
		const value = (body as Record<string, unknown>).completedAt;
		if (typeof value === 'string') return value;
	}
	return undefined;
}

/** Neutral defaults shared by every synthesized Task. */
function baseTask(id: number, now: string): Task {
	return {
		id,
		title: '',
		description: '',
		inboxId: null,
		contextId: null,
		projectId: null,
		sectionId: null,
		parentId: null,
		priority: 'no-priority',
		status: 'open',
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
		labels: [],
		url: '',
		createdAt: now,
		updatedAt: now
	};
}

/**
 * Minimal Task returned when the real one is not in cache (§4.5): the page has
 * already applied its own optimistic update via `useListMutator` and only needs
 * a success signal — replay + refetch reconcile the authoritative object later.
 */
export function minimalTask(
	id: number,
	status: TaskStatus,
	completedAt: string | null,
	now: string
): Task {
	return { ...baseTask(id, now), status, completedAt };
}

/**
 * Build the optimistic Task for an offline-created inbox task from its
 * `TaskInput` and a pre-minted negative `tmpId` (§4.5). `labels` stays empty:
 * offline we only have label *names*, not resolved `Label` objects; refetch after
 * replay populates them.
 */
export function taskFromInput(input: TaskInput, tmpId: number, now: string): Task {
	return {
		...baseTask(tmpId, now),
		title: input.title ?? '',
		description: input.description ?? '',
		priority: input.priority ?? 'no-priority',
		dueAt: input.dueAt ?? null,
		dueHasTime: input.dueHasTime ?? false,
		deadlineAt: input.deadlineAt ?? null,
		deadlineHasTime: input.deadlineHasTime ?? false,
		dayPart: input.dayPart ?? 'none',
		planState: input.planState ?? 'none',
		recurrenceRule: input.recurrenceRule ?? null,
		isPrivate: input.isPrivate ?? false,
		isComplex: input.isComplex ?? false
	};
}
