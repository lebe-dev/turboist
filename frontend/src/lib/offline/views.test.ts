import 'fake-indexeddb/auto';
import { describe, it, expect, beforeEach } from 'vitest';
import { TurboistDB, type StoredEntity } from './db';
import type { Task } from '../api/types';
import {
	queryToday,
	queryTomorrow,
	queryOverdue,
	queryInbox,
	queryBacklog,
	queryPinned,
	queryWeek,
	queryPlanStats,
	queryCompletedToday,
	queryProjectTasks,
	queryLabelTasks,
	searchTasks
} from './views';

const TZ = 'UTC';
const NOW = new Date('2026-05-15T12:00:00.000Z');

function task(overrides: Partial<Task>): Task {
	return {
		id: 1,
		title: 't',
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
		completedAt: null,
		recurrenceRule: null,
		postponeCount: 0,
		labels: [],
		url: '',
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z',
		...overrides
	};
}

async function putTask(db: TurboistDB, t: Task, deletedAt: string | null = null): Promise<void> {
	const entity: StoredEntity = {
		clientId: `c${t.id}`,
		serverId: t.id,
		updatedAt: t.updatedAt,
		deletedAt,
		data: t as unknown as Record<string, unknown>
	};
	await db.tasks.put(entity);
}

describe('offline/views', () => {
	let db: TurboistDB;

	beforeEach(async () => {
		db = new TurboistDB(`test-views-${Math.random().toString(36).slice(2)}`);
		await db.open();
	});

	it('queryToday returns open tasks due today, skipping completed and other days', async () => {
		await putTask(db, task({ id: 1, dueAt: '2026-05-15T08:00:00.000Z' }));
		await putTask(db, task({ id: 2, dueAt: '2026-05-15T23:00:00.000Z', status: 'completed' }));
		await putTask(db, task({ id: 3, dueAt: '2026-05-14T08:00:00.000Z' }));
		await putTask(db, task({ id: 4, dueAt: null }));

		const res = await queryToday(TZ, null, NOW, db);
		expect(res.items.map((t) => t.id)).toEqual([1]);
		expect(res.total).toBe(1);
	});

	it('queryTomorrow uses next day in tz', async () => {
		await putTask(db, task({ id: 10, dueAt: '2026-05-16T05:00:00.000Z' }));
		await putTask(db, task({ id: 11, dueAt: '2026-05-15T05:00:00.000Z' }));

		const res = await queryTomorrow(TZ, null, NOW, db);
		expect(res.items.map((t) => t.id)).toEqual([10]);
	});

	it('queryOverdue returns open tasks with due day before today', async () => {
		await putTask(db, task({ id: 20, dueAt: '2026-05-14T20:00:00.000Z' }));
		await putTask(db, task({ id: 21, dueAt: '2026-05-15T05:00:00.000Z' }));
		await putTask(db, task({ id: 22, dueAt: null }));

		const res = await queryOverdue(TZ, null, NOW, db);
		expect(res.items.map((t) => t.id)).toEqual([20]);
	});

	it('queryInbox returns open tasks with inboxId != null', async () => {
		await putTask(db, task({ id: 30, inboxId: 1 }));
		await putTask(db, task({ id: 31, inboxId: 1, status: 'completed' }));
		await putTask(db, task({ id: 32, inboxId: null }));

		const res = await queryInbox(null, db);
		expect(res.items.map((t) => t.id)).toEqual([30]);
	});

	it('queryBacklog filters by planState=backlog and open', async () => {
		await putTask(db, task({ id: 40, planState: 'backlog' }));
		await putTask(db, task({ id: 41, planState: 'week' }));
		await putTask(db, task({ id: 42, planState: 'backlog', status: 'completed' }));

		const res = await queryBacklog(null, db);
		expect(res.items.map((t) => t.id)).toEqual([40]);
	});

	it('queryPinned filters by isPinned and open', async () => {
		await putTask(db, task({ id: 50, isPinned: true }));
		await putTask(db, task({ id: 51, isPinned: false }));
		await putTask(db, task({ id: 52, isPinned: true, status: 'completed' }));

		const res = await queryPinned(null, db);
		expect(res.items.map((t) => t.id)).toEqual([50]);
	});

	it('queryWeek returns tasks planned for week or due within current week', async () => {
		// Week containing 2026-05-15 (Friday): Mon 2026-05-11 .. Mon 2026-05-18
		await putTask(db, task({ id: 60, planState: 'week' }));
		await putTask(db, task({ id: 61, dueAt: '2026-05-12T10:00:00.000Z' }));
		await putTask(db, task({ id: 62, dueAt: '2026-05-20T10:00:00.000Z' }));
		await putTask(db, task({ id: 63, status: 'completed', planState: 'week' }));

		const res = await queryWeek(TZ, null, NOW, db);
		const ids = res.items.map((t) => t.id).sort();
		expect(ids).toEqual([60, 61]);
		expect(res.plannedCount).toBe(1);
	});

	it('queryPlanStats counts open week + backlog tasks', async () => {
		await putTask(db, task({ id: 70, planState: 'week' }));
		await putTask(db, task({ id: 71, planState: 'week' }));
		await putTask(db, task({ id: 72, planState: 'backlog' }));
		await putTask(db, task({ id: 73, planState: 'backlog', status: 'completed' }));

		const stats = await queryPlanStats(null, db);
		expect(stats).toEqual({ week: 2, backlog: 1 });
	});

	it('queryCompletedToday returns tasks completed within window', async () => {
		await putTask(
			db,
			task({ id: 80, status: 'completed', completedAt: '2026-05-15T08:00:00.000Z' })
		);
		await putTask(
			db,
			task({ id: 81, status: 'completed', completedAt: '2026-05-14T08:00:00.000Z' })
		);

		const today = await queryCompletedToday(TZ, null, 1, NOW, db);
		expect(today.items.map((t) => t.id)).toEqual([80]);

		const twoDays = await queryCompletedToday(TZ, null, 2, NOW, db);
		const ids = twoDays.items.map((t) => t.id).sort();
		expect(ids).toEqual([80, 81]);
	});

	it('queryProjectTasks filters by projectId', async () => {
		await putTask(db, task({ id: 90, projectId: 5 }));
		await putTask(db, task({ id: 91, projectId: 6 }));

		const res = await queryProjectTasks(5, db);
		expect(res.items.map((t) => t.id)).toEqual([90]);
	});

	it('queryLabelTasks filters open tasks by label membership', async () => {
		const lbl = { id: 9, name: 'x', color: '#fff', isFavourite: false, isPrivate: false, createdAt: '', updatedAt: '' };
		await putTask(db, task({ id: 100, labels: [lbl] }));
		await putTask(db, task({ id: 101, labels: [] }));
		await putTask(db, task({ id: 102, labels: [lbl], status: 'completed' }));

		const res = await queryLabelTasks(9, db);
		expect(res.items.map((t) => t.id)).toEqual([100]);
	});

	it('searchTasks matches title or description case-insensitively', async () => {
		await putTask(db, task({ id: 110, title: 'Buy milk', description: '' }));
		await putTask(db, task({ id: 111, title: 'Sell house', description: 'with MILK fridge' }));
		await putTask(db, task({ id: 112, title: 'Other', description: '' }));

		const res = await searchTasks('milk', db);
		const ids = res.items.map((t) => t.id).sort();
		expect(ids).toEqual([110, 111]);
	});

	it('skips soft-deleted rows', async () => {
		await putTask(db, task({ id: 120 }), '2026-05-14T00:00:00.000Z');
		await putTask(db, task({ id: 121 }));

		const res = await queryInbox(null, db);
		expect(res.items.map((t) => t.id)).toEqual([]);
	});

	it('filters by contextId when provided', async () => {
		await putTask(db, task({ id: 130, contextId: 1, planState: 'backlog' }));
		await putTask(db, task({ id: 131, contextId: 2, planState: 'backlog' }));

		const res = await queryBacklog(1, db);
		expect(res.items.map((t) => t.id)).toEqual([130]);
	});
});
