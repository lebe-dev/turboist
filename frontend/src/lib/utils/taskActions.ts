import type { Label, Task, TaskInput, TaskMoveInput } from '$lib/api/types';
import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
import { getApiClient } from '$lib/api/client';
import { ApiError } from '$lib/api/errors';
import { refreshSidebarBundle } from '$lib/realtime/refresh';
import { pinnedTasksStore } from '$lib/stores/pinnedTasks.svelte';
import { followUpStore } from '$lib/stores/followUp.svelte';
import { configStore } from '$lib/stores/config.svelte';
import { soundStore } from '$lib/stores/sound.svelte';
import { labelsStore } from '$lib/stores/labels.svelte';
import {
	isOverdue,
	dayStartUtcInTz,
	toIsoUtc,
	dayKeyInTz,
	shiftDayKey,
	parseIso,
	timeKeyInTz
} from '$lib/utils/format';
import { nowStore } from '$lib/stores/now.svelte';
import { toast } from 'svelte-sonner';
import { get } from 'svelte/store';
import { t } from '$lib/i18n';
import { buildTree, type TaskNode } from '$lib/utils/taskTree';

function tr(key: string, values?: Record<string, string | number | Date>): string {
	return get(t)(key, values ? { values } : undefined);
}

export function describeError(err: unknown, fallback: string): string {
	if (err instanceof ApiError) {
		// A whitelisted mutation attempted while offline (client.ts, §4.4): the
		// synthetic `offline_unsupported` code carries an English message; map it to
		// the localized string instead of surfacing that raw text.
		if (err.code === 'offline_unsupported') return tr('offline.unsupported');
		// A network error / timeout that reached no cache (status 0) — e.g. opening a
		// project offline that was never visited online. WebKit surfaces a raw
		// "Load failed"; map it to a clear localized "needs connection" message.
		if (err.status === 0 && (err.code === 'network_error' || err.code === 'timeout')) {
			return tr('offline.needsConnection');
		}
		// A completion refused because the task still has open blockers. The client
		// normally guards this before sending, so reaching here means a stale view —
		// still worth a localized message rather than the backend's English one.
		if (err.code === 'task_blocked') return tr('task.toast.blockedCannotComplete');
		return err.message || fallback;
	}
	if (err instanceof Error) return err.message;
	return fallback;
}

/**
 * Whether a task cannot be completed because something still blocks it.
 * `blockedByCount` counts only OPEN blockers (the backend filters completed and
 * cancelled ones out), so this needs no extra status check on the peers.
 *
 * Optional-chained on the count: a Task synthesized by an older offline-cache entry
 * predates the field, and treating that as blocked would wedge the checkbox.
 */
export function isBlocked(task: Task): boolean {
	return (task.blockedByCount ?? 0) > 0;
}

export interface ListMutator {
	replace(task: Task): void;
	remove(id: number): void;
	// Drops the task together with its descendants. Optional: a mutator that cannot
	// see the surrounding list falls back to `remove` via `removeWithSubtree`.
	removeSubtree?: (id: number) => void;
	insertAfter?: (id: number, task: Task) => void;
	add?: (task: Task) => void;
}

/**
 * Remove a task from the view along with its subtree, for the operations the
 * server cascades down the tree (delete, park in backlog, plan for the week).
 * Falls back to a plain single-task removal where the mutator has no subtree
 * support — the stale subtasks then disappear on the next reload.
 */
function removeWithSubtree(mutator: ListMutator, id: number): void {
	if (mutator.removeSubtree) {
		mutator.removeSubtree(id);
		return;
	}
	mutator.remove(id);
}

export interface ToggleCompleteOptions {
	// On views that filter to open tasks (Today, Overdue, Backlog, …), a task
	// that just got completed is no longer in the result set, so drop it from
	// local state. On unfiltered views (Project, Context, Label, Search) it
	// must stay so the user can immediately uncomplete it.
	removeWhenCompleted?: boolean;
	// Predicate that decides whether the (still-open) updated task belongs in
	// the current view. Recurring tasks stay open after completion but get
	// their due_at advanced, which can move them out of date-bound views like
	// Today/Tomorrow/Overdue.
	belongs?: (task: Task) => boolean;
	// Override completion timestamp (ISO UTC). Used when completing an overdue
	// task to record the actual completion day chosen by the user.
	completedAt?: string;
}

export async function toggleComplete(
	task: Task,
	mutator: ListMutator,
	options: ToggleCompleteOptions = {}
): Promise<void> {
	const { removeWhenCompleted = true, belongs, completedAt } = options;
	const client = getApiClient();
	const wasOpen = task.status !== 'completed';
	// Single guard for every checkbox in the app: all list views, the detail page and
	// the completed-footer cards route their toggle through here. The backend refuses
	// it too (409 task_blocked) — this keeps the refusal instant and offline-safe.
	// Uncompleting is never blocked, hence the wasOpen check.
	if (wasOpen && isBlocked(task)) {
		toast.error(tr('task.toast.blockedCannotComplete'));
		return;
	}
	try {
		const updated = wasOpen
			? await tasksApi.complete(client, task.id, completedAt)
			: await tasksApi.uncomplete(client, task.id);
		soundStore.playTaskStatus(updated.status === 'completed');
		if (updated.status === 'completed' && removeWhenCompleted) mutator.remove(task.id);
		else if (updated.status !== 'completed' && belongs && !belongs(updated)) mutator.remove(task.id);
		else mutator.replace(updated);

		if (wasOpen && updated.status === 'completed' && !updated.recurrenceRule) {
			const undoFn = async () => {
				const restored = await tasksApi.uncomplete(client, task.id);
				soundStore.playTaskStatus(false);
				if (removeWhenCompleted) {
					if (mutator.add) mutator.add(restored);
					else mutator.replace(restored);
				} else {
					mutator.replace(restored);
				}
			};
			followUpStore.push(updated, undoFn);
		}
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedUpdate')));
	}
}

export async function togglePin(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	const wasPin = task.isPinned;
	try {
		const updated = wasPin
			? await tasksApi.unpin(client, task.id)
			: await tasksApi.pin(client, task.id);
		mutator.replace(updated);
		if (wasPin) pinnedTasksStore.removeItem(task.id);
		else pinnedTasksStore.addItem(updated);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedPin')));
	}
}

export async function deleteTask(task: Task, mutator: ListMutator): Promise<void> {
	const client = getApiClient();
	if (!confirm(tr('task.toast.confirmDelete', { title: task.title }))) return;
	try {
		await tasksApi.remove(client, task.id);
		// The delete cascades down the whole subtree server-side, so the subtasks are
		// gone too — leaving their rows behind would show tasks that no longer exist.
		removeWithSubtree(mutator, task.id);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedDelete')));
	}
}

export interface BelongsOption {
	belongs?: (task: Task) => boolean;
}

function applyUpdate(updated: Task, mutator: ListMutator, belongs?: (t: Task) => boolean): void {
	if (belongs && !belongs(updated)) mutator.remove(updated.id);
	else mutator.replace(updated);
}

export async function updateTaskFields(
	task: Task,
	mutator: ListMutator,
	patch: TaskInput,
	options: BelongsOption = {}
): Promise<void> {
	const client = getApiClient();
	const tz = configStore.value?.timezone ?? null;
	// Changing the day-part of an overdue task implies the user wants to act on
	// it today, so reschedule its due_at to the current day instead of leaving
	// it stuck in the past.
	if (patch.dayPart !== undefined && patch.dueAt === undefined && isOverdue(task.dueAt, tz)) {
		const todayKey = nowStore.todayKey;
		patch = {
			...patch,
			dueAt: toIsoUtc(dayStartUtcInTz(todayKey, tz)),
			dueHasTime: false
		};
	}
	try {
		const updated = await tasksApi.update(client, task.id, patch);
		applyUpdate(updated, mutator, options.belongs);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedUpdate')));
	}
}

export async function moveToBacklog(
	task: Task,
	mutator: ListMutator,
	options: BelongsOption = {}
): Promise<void> {
	const client = getApiClient();
	try {
		if (task.dueAt) {
			await tasksApi.update(client, task.id, { dueAt: null, dueHasTime: false });
		}
		const updated = await tasksApi.plan(client, task.id, { state: 'backlog' });
		// Subtasks are parked alongside their parent (service.PlanService.SetPlanState),
		// so once the parent leaves this view its subtree leaves with it.
		if (options.belongs && !options.belongs(updated)) removeWithSubtree(mutator, updated.id);
		else mutator.replace(updated);
		void refreshSidebarBundle().catch(() => {});
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedMoveToBacklog')));
	}
}

export async function removeFromBacklog(
	task: Task,
	mutator: ListMutator,
	options: BelongsOption = {}
): Promise<void> {
	const client = getApiClient();
	try {
		const updated = await tasksApi.plan(client, task.id, { state: 'none' });
		applyUpdate(updated, mutator, options.belongs);
		void refreshSidebarBundle().catch(() => {});
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedRemoveFromBacklog')));
	}
}

const TITLE_COUNTER = /^.+ \(\d+\)$/;

/** True when the title ends in a counter like `Read a chapter (3)`. */
export function hasTitleCounter(title: string): boolean {
	return TITLE_COUNTER.test(title);
}

/**
 * Due date for a copy placed on tomorrow: the start of tomorrow, or the same
 * time of day when the source is timed.
 */
export function tomorrowDuplicateDue(
	dueAt: string | null,
	dueHasTime: boolean,
	now: Date,
	tz: string | null
): { dueAt: string; dueHasTime: boolean } {
	const start = dayStartUtcInTz(shiftDayKey(dayKeyInTz(now, tz), 1), tz);
	const source = dueHasTime ? parseIso(dueAt) : null;
	if (!source) return { dueAt: toIsoUtc(start), dueHasTime: false };
	const [hours, minutes] = timeKeyInTz(source, tz).split(':').map(Number);
	return {
		dueAt: toIsoUtc(new Date(start.getTime() + (hours * 60 + minutes) * 60000)),
		dueHasTime: true
	};
}

export async function duplicateTask(
	task: Task,
	mutator: ListMutator,
	options: { toTomorrow?: boolean; belongs?: (t: Task) => boolean } = {}
): Promise<void> {
	const client = getApiClient();
	try {
		const due = options.toTomorrow
			? tomorrowDuplicateDue(
					task.dueAt,
					task.dueHasTime,
					nowStore.now,
					configStore.value?.timezone ?? null
				)
			: undefined;
		const created = await tasksApi.duplicate(client, task.id, due);
		// A copy dated tomorrow may not belong in the list on screen; it shows up
		// where it belongs on the next load.
		if (!options.belongs || options.belongs(created)) {
			// Insert the clone plus its cloned subtasks as a flat run so the tree
			// view can nest them by parentId without waiting for a reload. Chain off
			// each inserted id to preserve subtask order.
			let afterId = task.id;
			mutator.insertAfter?.(afterId, created);
			afterId = created.id;
			for (const sub of created.subtasks?.items ?? []) {
				mutator.insertAfter?.(afterId, sub);
				afterId = sub.id;
			}
		}
		toast.success(
			tr(options.toTomorrow ? 'task.toast.duplicatedTomorrow' : 'task.toast.duplicated')
		);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedDuplicate')));
	}
}

function restoreInput(task: Task): TaskMoveInput {
	if (task.inboxId !== null) return { inboxId: task.inboxId };
	const input: { contextId: number; projectId?: number; sectionId?: number } = {
		contextId: task.contextId as number
	};
	if (task.projectId !== null) input.projectId = task.projectId;
	if (task.sectionId !== null) input.sectionId = task.sectionId;
	return input;
}

function pushMoveUndo(
	original: Task,
	mutator: ListMutator,
	destination: string,
	wasJustCompleted: boolean
): void {
	const client = getApiClient();
	const undoFn = async () => {
		let restored = await tasksApi.move(client, original.id, restoreInput(original));
		if (wasJustCompleted && restored.status === 'completed') {
			restored = await tasksApi.uncomplete(client, original.id);
		}
		if (mutator.add) mutator.add(restored);
		else mutator.replace(restored);
	};
	followUpStore.pushMove(original.title, destination, undoFn);
}

export async function moveTaskToInbox(
	task: Task,
	mutator: ListMutator,
	options: BelongsOption = {}
): Promise<void> {
	if (task.inboxId !== null) return;
	const client = getApiClient();
	const original = task;
	try {
		const updated = await tasksApi.move(client, task.id, { inboxId: 1 });
		applyUpdate(updated, mutator, options.belongs);
		pushMoveUndo(original, mutator, tr('nav.inbox'), false);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedMove')));
	}
}

export async function moveTaskToProject(
	task: Task,
	contextId: number,
	projectId: number,
	mutator: ListMutator,
	options: BelongsOption & {
		projectCompleted?: boolean;
		sectionId?: number | null;
		projectName?: string;
		sectionName?: string | null;
	} = {}
): Promise<void> {
	if (
		task.projectId === projectId &&
		(options.sectionId === undefined || task.sectionId === (options.sectionId ?? null))
	)
		return;
	const client = getApiClient();
	const original = task;
	try {
		const moveInput =
			options.sectionId != null
				? { contextId, projectId, sectionId: options.sectionId }
				: { contextId, projectId };
		let updated = await tasksApi.move(client, task.id, moveInput);
		// Moving into a completed project auto-completes the task — unless something
		// still blocks it. Skipping the completion (rather than letting the 409 bubble)
		// keeps the move itself, which already succeeded, instead of reporting the
		// whole operation as failed.
		const completedHere =
			!!options.projectCompleted && updated.status !== 'completed' && !isBlocked(updated);
		if (completedHere) {
			updated = await tasksApi.complete(client, updated.id);
		} else if (!!options.projectCompleted && isBlocked(updated)) {
			toast.warning(tr('task.toast.blockedNotCompletedOnMove'));
		}
		applyUpdate(updated, mutator, options.belongs);
		const destination = options.projectName
			? options.sectionName
				? `${options.projectName} › ${options.sectionName}`
				: options.projectName
			: tr('task.toast.moved');
		pushMoveUndo(original, mutator, destination, completedHere);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedMove')));
	}
}

export async function moveTaskToSection(
	task: Task,
	contextId: number,
	projectId: number,
	sectionId: number | null,
	mutator: ListMutator,
	options: BelongsOption & { projectName?: string; sectionName?: string | null } = {}
): Promise<void> {
	if (task.sectionId === sectionId) return;
	const client = getApiClient();
	const original = task;
	try {
		const moveInput =
			sectionId !== null ? { contextId, projectId, sectionId } : { contextId, projectId };
		const updated = await tasksApi.move(client, task.id, moveInput);
		applyUpdate(updated, mutator, options.belongs);
		const destination = options.projectName
			? options.sectionName
				? `${options.projectName} › ${options.sectionName}`
				: options.projectName
			: tr('task.toast.movedToSection');
		pushMoveUndo(original, mutator, destination, false);
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedMoveToSection')));
	}
}

export async function decomposeTask(
	task: Task,
	titles: string[],
	mutator: ListMutator
): Promise<boolean> {
	const client = getApiClient();
	try {
		const res = await tasksApi.decompose(client, task.id, titles);
		mutator.remove(task.id);
		for (const t of res.created) {
			if (mutator.add) mutator.add(t);
			else mutator.replace(t);
		}
		toast.success(tr('task.toast.decomposed', { count: res.created.length }));
		return true;
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedDecompose')));
		return false;
	}
}

// Bulk action: overwrite every direct subtask's labels with the parent's
// labels. Replaces each subtask's label set (it does not merge) and reflects
// the updated subtasks back into the current view.
export async function applyParentLabelsToSubtasks(
	parent: Task,
	mutator: ListMutator
): Promise<void> {
	const client = getApiClient();
	try {
		const { items } = await tasksApi.listSubtasks(client, parent.id);
		if (items.length === 0) {
			toast.success(tr('task.toast.subtaskLabelsApplied', { count: 0 }));
			return;
		}
		const labels = parent.labels.map((l) => l.name);
		const updated = await Promise.all(
			items.map((sub) => tasksApi.update(client, sub.id, { labels }))
		);
		for (const sub of updated) mutator.replace(sub);
		toast.success(tr('task.toast.subtaskLabelsApplied', { count: updated.length }));
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedApplySubtaskLabels')));
	}
}

export async function copyTaskTitle(task: Task): Promise<void> {
	try {
		await navigator.clipboard.writeText(task.title);
		toast.success(tr('task.toast.copied'));
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedCopy')));
	}
}

export interface TaskJson {
	title: string;
	description: string;
	/** Names of the labels on this task; their meaning lives in the root glossary. */
	labels: string[];
	subtasks?: TaskJson[];
}

export interface LabelGlossaryEntry {
	name: string;
	description: string;
}

/** The copied tree plus one glossary entry per label used anywhere in it. */
export interface RootTaskJson extends TaskJson {
	labelGlossary: LabelGlossaryEntry[];
}

/**
 * Serialises a task and its subtree for the clipboard. Each task names its
 * labels; the root carries a glossary describing every label once, so a reader
 * such as an AI agent learns what a label means without repeated text.
 * `knownLabels` supplies fresher label descriptions than the ones embedded in
 * the tasks, e.g. right after a label was edited.
 */
export function taskToJson(
	task: Task,
	descendants: Task[],
	knownLabels: Label[] = []
): RootTaskJson {
	const root = buildTree([task, ...descendants]).find((node) => node.task.id === task.id)!;
	const known = new Map(knownLabels.map((label) => [label.id, label]));
	const glossary = new Map<string, string>();
	const serialize = (node: TaskNode): TaskJson => {
		const labels = node.task.labels ?? [];
		for (const label of labels) {
			glossary.set(label.name, known.get(label.id)?.description ?? label.description ?? '');
		}
		return {
			title: node.task.title,
			description: node.task.description,
			labels: labels.map((label) => label.name),
			...(node.children.length > 0 ? { subtasks: node.children.map(serialize) } : {})
		};
	};
	const tree = serialize(root);
	const labelGlossary = [...glossary]
		.sort(([a], [b]) => compareNames(a, b))
		.map(([name, description]) => ({ name, description }));
	return { ...tree, labelGlossary };
}

/** Case-insensitive name order, the same order the Android copy uses. */
function compareNames(a: string, b: string): number {
	const la = a.toLowerCase();
	const lb = b.toLowerCase();
	if (la !== lb) return la < lb ? -1 : 1;
	return a < b ? -1 : a > b ? 1 : 0;
}

export async function copyTaskJson(task: Task): Promise<void> {
	try {
		const text = tasksApi
			.get(getApiClient(), task.id, { includeSubtasks: true })
			.then((current) =>
				JSON.stringify(
					taskToJson(current, current.subtasks?.items ?? [], labelsStore.items),
					null,
					2
				)
			);
		if (typeof ClipboardItem !== 'undefined' && navigator.clipboard.write) {
			await navigator.clipboard.write([
				new ClipboardItem({
					'text/plain': text.then((value) => new Blob([value], { type: 'text/plain' }))
				})
			]);
		} else {
			await navigator.clipboard.writeText(await text);
		}
		toast.success(tr('task.toast.jsonCopied'));
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedCopy')));
	}
}

/** Copies the bare numeric id — what the relation picker and the API both take. */
export async function copyTaskId(task: Task): Promise<void> {
	try {
		await navigator.clipboard.writeText(String(task.id));
		toast.success(tr('task.toast.idCopied'));
	} catch (err) {
		toast.error(describeError(err, tr('task.toast.failedCopy')));
	}
}
