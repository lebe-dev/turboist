import { getDB, type TurboistDB } from './db';
import { onOutboxChanged } from './stores';
import { flush, type SyncFetch } from './sync';

const FALLBACK_INTERVAL_MS = 30_000;

class SyncRunner {
	#started = false;
	#unsubs: Array<() => void> = [];
	#timer: ReturnType<typeof setInterval> | null = null;
	#fetcher: SyncFetch | null = null;
	#db: TurboistDB | null = null;

	start(fetcher: SyncFetch, db: TurboistDB = getDB()): void {
		if (this.#started) return;
		this.#started = true;
		this.#fetcher = fetcher;
		this.#db = db;

		if (typeof window !== 'undefined') {
			const onOnline = (): void => {
				void this.notify();
			};
			window.addEventListener('online', onOnline);
			this.#unsubs.push(() => window.removeEventListener('online', onOnline));
		}
		if (typeof document !== 'undefined') {
			const onVisible = (): void => {
				if (document.visibilityState === 'visible') void this.notify();
			};
			document.addEventListener('visibilitychange', onVisible);
			this.#unsubs.push(() => document.removeEventListener('visibilitychange', onVisible));
		}

		this.#unsubs.push(onOutboxChanged(() => void this.notify()));
		this.#timer = setInterval(() => void this.notify(), FALLBACK_INTERVAL_MS);
		void this.notify();
	}

	async notify(): Promise<void> {
		if (!this.#fetcher || !this.#db) return;
		if (typeof navigator !== 'undefined' && navigator.onLine === false) return;
		try {
			await flush(this.#fetcher, this.#db);
		} catch (err) {
			console.warn('syncRunner: flush failed', err);
		}
	}

	stop(): void {
		if (!this.#started) return;
		this.#started = false;
		for (const u of this.#unsubs) u();
		this.#unsubs = [];
		if (this.#timer !== null) {
			clearInterval(this.#timer);
			this.#timer = null;
		}
		this.#fetcher = null;
		this.#db = null;
	}
}

export const syncRunner = new SyncRunner();
