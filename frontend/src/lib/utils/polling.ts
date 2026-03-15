export interface PollingOptions {
	/** Interval in milliseconds */
	interval: number;
	/** Called on each tick (and immediately on start) */
	fn: () => Promise<void>;
	/** Called on error (except 401 which stops polling) */
	onError?: (err: unknown) => void;
}

export interface Poller {
	start(): void;
	stop(): void;
	setInterval(ms: number): void;
}

export function createPoller(options: PollingOptions): Poller {
	let intervalMs = options.interval;
	let timerId: ReturnType<typeof setTimeout> | null = null;
	let running = false;

	function schedule(): void {
		timerId = setTimeout(tick, intervalMs);
	}

	async function tick(): Promise<void> {
		if (!running) return;

		try {
			await options.fn();
		} catch (err) {
			// 401 is handled in client.ts (goto /login) — stop polling
			if (err instanceof Error && err.message.startsWith('401')) {
				stop();
				return;
			}
			options.onError?.(err);
		}

		if (running) schedule();
	}

	function onVisibilityChange(): void {
		if (document.hidden) {
			if (timerId !== null) {
				clearTimeout(timerId);
				timerId = null;
			}
		} else if (running) {
			// Immediate tick on tab re-focus
			tick();
		}
	}

	function start(): void {
		if (running) return;
		running = true;
		document.addEventListener('visibilitychange', onVisibilityChange);
		tick();
	}

	function stop(): void {
		running = false;
		if (timerId !== null) {
			clearTimeout(timerId);
			timerId = null;
		}
		document.removeEventListener('visibilitychange', onVisibilityChange);
	}

	function setInterval(ms: number): void {
		intervalMs = ms;
	}

	return { start, stop, setInterval };
}
