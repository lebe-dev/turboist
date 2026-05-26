import { tasks as tasksApi } from '../api/endpoints/tasks';
import { getApiClient } from '../api/client';
import { cacheStoreValue, getCachedStoreValue } from '../offline/storeCache';

const CACHE_KEY = 'inboxStats';

interface InboxStatsSnapshot {
	count: number;
	warnThresholdExceeded: boolean;
}

class InboxStatsStore {
	count = $state<number>(0);
	warnThresholdExceeded = $state<boolean>(false);

	async load(): Promise<void> {
		const res = await tasksApi.inbox(getApiClient());
		this.count = res.count;
		this.warnThresholdExceeded = res.warnThresholdExceeded;
		void cacheStoreValue(CACHE_KEY, {
			count: res.count,
			warnThresholdExceeded: res.warnThresholdExceeded
		} satisfies InboxStatsSnapshot).catch(() => undefined);
	}

	async loadCached(): Promise<boolean> {
		const cached = await getCachedStoreValue<InboxStatsSnapshot>(CACHE_KEY);
		if (!cached) return false;
		this.count = cached.count;
		this.warnThresholdExceeded = cached.warnThresholdExceeded;
		return true;
	}

	set(count: number, warnThresholdExceeded: boolean): void {
		this.count = count;
		this.warnThresholdExceeded = warnThresholdExceeded;
	}

	clear(): void {
		this.count = 0;
		this.warnThresholdExceeded = false;
	}
}

export const inboxStatsStore = new InboxStatsStore();
