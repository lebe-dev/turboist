export interface PollingOptions {
	/** Интервал в миллисекундах */
	interval: number;
	/** Вызывается при каждом тике (и сразу при старте) */
	fn: () => Promise<void>;
	/** Вызывается при ошибке (кроме 401 — он останавливает polling) */
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
			// 401 обрабатывается в client.ts (goto /login) — останавливаем polling
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
			// Немедленный тик при возврате на вкладку
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
