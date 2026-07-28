import 'fake-indexeddb/auto';
import { describe, expect, it, vi } from 'vitest';

// The `idb` handle is faked here (unlike db.test.ts, which runs against
// fake-indexeddb) because the point is a transaction that fails the way WebKit
// fails one on iOS: both the request and `tx.done` reject with `UnknownError`.
const { idbOpen } = vi.hoisted(() => ({ idbOpen: vi.fn() }));
vi.mock('idb', () => ({ openDB: idbOpen }));

import { openOfflineDB, type CachedResponse } from './db';

const SERVER = 'https://a.example.com';

function unknownError(): DOMException {
	return new DOMException(
		'An internal error was encountered in the Indexed Database server',
		'UnknownError'
	);
}

/**
 * Minimal `idb` handle: `meta` reads/writes succeed (so `openOfflineDB` gets
 * through reconciliation), every transaction fails.
 */
function createFakeHandle() {
	const meta = new Map<string, unknown>();
	let transactions = 0;
	return {
		get transactions() {
			return transactions;
		},
		handle: {
			async get(store: string, key: string) {
				return store === 'meta' ? meta.get(key) : undefined;
			},
			async put(store: string, value: { key: string; value: unknown }) {
				if (store === 'meta') meta.set(value.key, value);
				return value.key;
			},
			async getAll() {
				return [];
			},
			transaction() {
				transactions += 1;
				return {
					// Rejected up front, exactly like a transaction the browser aborted.
					done: Promise.reject(unknownError()),
					store: {
						put: () => Promise.reject(unknownError()),
						count: () => Promise.resolve(0),
						index: () => ({ openCursor: () => Promise.resolve(null) })
					}
				};
			},
			close() {}
		}
	};
}

function entry(): CachedResponse {
	return {
		cacheKey: '/api/v1/tasks/today',
		payload: { items: [] },
		storedAt: '2026-01-01T00:00:00.000Z',
		path: '/api/v1/tasks/today'
	};
}

describe('failing transactions', () => {
	it('rejects the caller without leaving tx.done unobserved', async () => {
		const fake = createFakeHandle();
		idbOpen.mockResolvedValue(fake.handle);

		const rejections: unknown[] = [];
		const onUnhandled = (reason: unknown): void => {
			rejections.push(reason);
		};
		process.on('unhandledRejection', onUnhandled);
		try {
			const db = await openOfflineDB(SERVER);
			await expect(db.putResponse(entry())).rejects.toMatchObject({ name: 'UnknownError' });
			// Give the runtime a turn to report any unhandled rejection.
			await new Promise((resolve) => setTimeout(resolve, 20));
		} finally {
			process.off('unhandledRejection', onUnhandled);
		}

		expect(rejections).toEqual([]);
		// UnknownError is treated as a lost connection: one reopen-and-retry.
		expect(fake.transactions).toBe(2);
	});
});
