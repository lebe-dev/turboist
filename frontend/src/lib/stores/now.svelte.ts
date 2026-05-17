import { configStore } from './config.svelte';
import { dayKeyInTz, dayStartUtcInTz, shiftDayKey } from '$lib/utils/format';

function createNowStore() {
	let now = $state<Date>(new Date());
	let midnightTimer: ReturnType<typeof setTimeout> | null = null;

	function clearTimer(): void {
		if (midnightTimer !== null) {
			clearTimeout(midnightTimer);
			midnightTimer = null;
		}
	}

	function scheduleMidnight(): void {
		clearTimer();
		const tz = configStore.value?.timezone ?? null;
		const currentKey = dayKeyInTz(now, tz);
		const nextMidnight = dayStartUtcInTz(shiftDayKey(currentKey, 1), tz);
		// Buffer 500ms past midnight to avoid landing right on the boundary and
		// re-reading the previous day due to clock drift.
		const delay = Math.max(1_000, nextMidnight.getTime() - Date.now() + 500);
		midnightTimer = setTimeout(refresh, delay);
	}

	function refresh(): void {
		now = new Date();
		scheduleMidnight();
	}

	return {
		get now(): Date {
			return now;
		},
		get todayKey(): string {
			return dayKeyInTz(now, configStore.value?.timezone ?? null);
		},
		refresh,
		scheduleMidnight,
		teardown(): void {
			clearTimer();
		}
	};
}

export const nowStore = createNowStore();
