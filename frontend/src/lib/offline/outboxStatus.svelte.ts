import { ENTITY_TABLES, getDB, type OutboxEntry } from './db';
import { onDbChanged } from './stores';

const SCAN_INTERVAL_MS = 3000;

class OutboxStatusStore {
	online = $state(true);
	pending = $state(0);
	failed = $state<OutboxEntry[]>([]);

	#started = false;
	#timer: ReturnType<typeof setInterval> | null = null;
	#unsubs: Array<() => void> = [];
	#onOnline = (): void => {
		this.online = true;
		void this.refresh();
	};
	#onOffline = (): void => {
		this.online = false;
	};

	async refresh(): Promise<void> {
		try {
			const db = getDB();
			this.pending = await db.outbox.where('status').anyOf('pending', 'inflight').count();
			const failed = await db.outbox.where('status').equals('failed').toArray();
			this.failed = failed.sort((a, b) => a.createdAt - b.createdAt);
		} catch {
			// Dexie not ready (e.g. SSR/test without IDB) — keep previous values.
		}
	}

	start(): void {
		if (this.#started || typeof window === 'undefined') return;
		this.#started = true;
		this.online = typeof navigator === 'undefined' ? true : navigator.onLine;
		window.addEventListener('online', this.#onOnline);
		window.addEventListener('offline', this.#onOffline);
		for (const kind of ENTITY_TABLES) {
			this.#unsubs.push(onDbChanged(kind, () => void this.refresh()));
		}
		this.#timer = setInterval(() => void this.refresh(), SCAN_INTERVAL_MS);
		void this.refresh();
	}

	stop(): void {
		if (!this.#started) return;
		this.#started = false;
		window.removeEventListener('online', this.#onOnline);
		window.removeEventListener('offline', this.#onOffline);
		for (const u of this.#unsubs) u();
		this.#unsubs = [];
		if (this.#timer !== null) {
			clearInterval(this.#timer);
			this.#timer = null;
		}
	}

	async retry(id: string): Promise<void> {
		const db = getDB();
		await db.outbox.update(id, {
			status: 'pending',
			attempts: 0,
			nextAttemptAt: Date.now(),
			lastError: null
		});
		await this.refresh();
	}

	async discard(id: string): Promise<void> {
		const db = getDB();
		await db.outbox.delete(id);
		await this.refresh();
	}
}

export const outboxStatusStore = new OutboxStatusStore();
