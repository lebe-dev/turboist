import { describe, expect, it } from 'vitest';
import { matchOp, offlineOps } from './registry';

describe('offlineOps registry', () => {
	it('registers the three v1 ops', () => {
		expect(offlineOps.map((o) => o.type)).toEqual([
			'task.complete',
			'task.uncomplete',
			'task.createInbox'
		]);
	});
});

describe('matchOp routing', () => {
	it('routes POST /tasks/:id/complete to task.complete with its payload', () => {
		const r = matchOp('/api/v1/tasks/5/complete', 'POST', undefined);
		expect(r?.kind).toBe('op');
		if (r?.kind === 'op') {
			expect(r.op.type).toBe('task.complete');
			expect(r.payload).toEqual({ taskId: 5 });
		}
	});

	it('carries completedAt from the body into the complete payload', () => {
		const r = matchOp('/api/v1/tasks/5/complete', 'POST', {
			completedAt: '2026-01-01T00:00:00.000Z'
		});
		expect(r?.kind === 'op' && r.payload).toEqual({
			taskId: 5,
			completedAt: '2026-01-01T00:00:00.000Z'
		});
	});

	it('routes POST /tasks/:id/uncomplete to task.uncomplete', () => {
		const r = matchOp('/api/v1/tasks/7/uncomplete', 'POST', undefined);
		expect(r?.kind === 'op' && r.op.type).toBe('task.uncomplete');
		expect(r?.kind === 'op' && r.payload).toEqual({ taskId: 7 });
	});

	it('routes POST /inbox/tasks to task.createInbox capturing the input', () => {
		const r = matchOp('/api/v1/inbox/tasks', 'POST', { title: 'x' });
		expect(r?.kind === 'op' && r.op.type).toBe('task.createInbox');
		expect(r?.kind === 'op' && r.payload).toEqual({ input: { title: 'x' } });
	});

	it('blocks operations targeting a tmp (id < 0) task', () => {
		expect(matchOp('/api/v1/tasks/-3/complete', 'POST', undefined)).toEqual({ kind: 'blocked' });
		expect(matchOp('/api/v1/tasks/-3/uncomplete', 'POST', undefined)).toEqual({ kind: 'blocked' });
	});

	it('ignores a trailing query string', () => {
		expect(matchOp('/api/v1/tasks/5/complete?foo=1', 'POST', undefined)?.kind).toBe('op');
	});

	it('returns null for requests no op handles', () => {
		expect(matchOp('/api/v1/tasks/5', 'PATCH', {})).toBeNull(); // update — not queueable
		expect(matchOp('/api/v1/tasks/5/pin', 'POST', undefined)).toBeNull(); // pin — not whitelisted
		expect(matchOp('/api/v1/tasks/5/complete', 'GET', undefined)).toBeNull(); // wrong method
		expect(matchOp('/api/v1/inbox/tasks', 'GET', undefined)).toBeNull();
	});
});
