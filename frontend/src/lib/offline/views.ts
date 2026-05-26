import { getDB, type TurboistDB } from './db';
import type { Project, ProjectSection, Task, ViewList } from '../api/types';
import { compareTaskOrder } from '../utils/priority';
import {
	dayKeyInTz,
	parseIso,
	shiftDayKey,
	weekRangeKeys
} from '../utils/format';

interface BaseFilter {
	status?: Task['status'];
	contextId?: number | null;
}

function baseFilter(task: Task, opts: BaseFilter): boolean {
	if (opts.status && task.status !== opts.status) return false;
	if (opts.contextId !== undefined && opts.contextId !== null) {
		if (task.contextId !== opts.contextId) return false;
	}
	return true;
}

function sortTasks(tasks: Task[]): Task[] {
	return [...tasks].sort(compareTaskOrder);
}

function pack(items: Task[]): ViewList<Task> {
	return { items: sortTasks(items), total: items.length };
}

async function allEntities<T>(kind: string, db: TurboistDB): Promise<T[]> {
	const rows = await db.table(kind).toArray();
	const out: T[] = [];
	for (const row of rows) {
		if (row.deletedAt !== null) continue;
		out.push(row.data as T);
	}
	return out;
}

export async function allTasks(db: TurboistDB = getDB()): Promise<Task[]> {
	return allEntities<Task>('tasks', db);
}

export async function allProjects(db: TurboistDB = getDB()): Promise<Project[]> {
	return allEntities<Project>('projects', db);
}

export async function allSections(db: TurboistDB = getDB()): Promise<ProjectSection[]> {
	return allEntities<ProjectSection>('sections', db);
}

function dueDayKey(task: Task, tz: string | null): string | null {
	const dt = parseIso(task.dueAt);
	if (!dt) return null;
	return dayKeyInTz(dt, tz);
}

function completedDayKey(task: Task, tz: string | null): string | null {
	const dt = parseIso(task.completedAt);
	if (!dt) return null;
	return dayKeyInTz(dt, tz);
}

export async function queryToday(
	tz: string | null,
	contextId?: number | null,
	now: Date = new Date(),
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const todayKey = dayKeyInTz(now, tz);
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'open', contextId })) return false;
		return dueDayKey(t, tz) === todayKey;
	});
	return pack(items);
}

export async function queryTomorrow(
	tz: string | null,
	contextId?: number | null,
	now: Date = new Date(),
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const tomorrowKey = shiftDayKey(dayKeyInTz(now, tz), 1);
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'open', contextId })) return false;
		return dueDayKey(t, tz) === tomorrowKey;
	});
	return pack(items);
}

export async function queryOverdue(
	tz: string | null,
	contextId?: number | null,
	now: Date = new Date(),
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const todayKey = dayKeyInTz(now, tz);
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'open', contextId })) return false;
		const k = dueDayKey(t, tz);
		return k !== null && k < todayKey;
	});
	return pack(items);
}

export async function queryCompletedToday(
	tz: string | null,
	contextId?: number | null,
	days: number = 1,
	now: Date = new Date(),
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const todayKey = dayKeyInTz(now, tz);
	const startKey = shiftDayKey(todayKey, -Math.max(0, days - 1));
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'completed', contextId })) return false;
		const k = completedDayKey(t, tz);
		return k !== null && k >= startKey && k <= todayKey;
	});
	return pack(items);
}

export async function queryInbox(
	contextId?: number | null,
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'open', contextId })) return false;
		return t.inboxId !== null;
	});
	return pack(items);
}

export async function queryWeek(
	tz: string | null,
	contextId?: number | null,
	now: Date = new Date(),
	db: TurboistDB = getDB()
): Promise<ViewList<Task> & { plannedCount: number }> {
	const tasks = await allTasks(db);
	const range = weekRangeKeys(now, tz);
	let plannedCount = 0;
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'open', contextId })) return false;
		const dueKey = dueDayKey(t, tz);
		const inWeek = dueKey !== null && dueKey >= range.startKey && dueKey < range.endKey;
		const planned = t.planState === 'week';
		if (planned) plannedCount += 1;
		return planned || inWeek;
	});
	return { ...pack(items), plannedCount };
}

export async function queryBacklog(
	contextId?: number | null,
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'open', contextId })) return false;
		return t.planState === 'backlog';
	});
	return pack(items);
}

export async function queryPinned(
	contextId?: number | null,
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const items = tasks.filter((t) => {
		if (!baseFilter(t, { status: 'open', contextId })) return false;
		return t.isPinned;
	});
	return pack(items);
}

export async function queryPlanStats(
	contextId?: number | null,
	db: TurboistDB = getDB()
): Promise<{ week: number; backlog: number }> {
	const tasks = await allTasks(db);
	let week = 0;
	let backlog = 0;
	for (const t of tasks) {
		if (!baseFilter(t, { status: 'open', contextId })) continue;
		if (t.planState === 'week') week += 1;
		else if (t.planState === 'backlog') backlog += 1;
	}
	return { week, backlog };
}

export async function queryProjectTasks(
	projectId: number,
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const items = tasks.filter((t) => t.projectId === projectId);
	return pack(items);
}

export async function queryProjectSections(
	projectId: number,
	db: TurboistDB = getDB()
): Promise<ProjectSection[]> {
	const all = await allSections(db);
	return all
		.filter((s) => s.projectId === projectId)
		.sort((a, b) => a.position - b.position);
}

export async function queryLabelTasks(
	labelId: number,
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const tasks = await allTasks(db);
	const items = tasks.filter((t) => {
		if (t.status !== 'open') return false;
		return t.labels.some((l) => l.id === labelId);
	});
	return pack(items);
}

export async function searchTasks(
	query: string,
	db: TurboistDB = getDB()
): Promise<ViewList<Task>> {
	const q = query.trim().toLowerCase();
	if (q.length === 0) return { items: [], total: 0 };
	const tasks = await allTasks(db);
	const items = tasks.filter(
		(t) => t.title.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)
	);
	return pack(items);
}

export async function searchProjects(
	query: string,
	db: TurboistDB = getDB()
): Promise<ViewList<Project>> {
	const q = query.trim().toLowerCase();
	if (q.length === 0) return { items: [], total: 0 };
	const projects = await allProjects(db);
	const items = projects.filter(
		(p) => p.title.toLowerCase().includes(q) || p.description.toLowerCase().includes(q)
	);
	return { items, total: items.length };
}
