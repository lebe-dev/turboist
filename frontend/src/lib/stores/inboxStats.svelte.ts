import { tasks as tasksApi } from '../api/endpoints/tasks';
import { getApiClient } from '../api/client';

class InboxStatsStore {
	count = $state<number>(0);
	warnThresholdExceeded = $state<boolean>(false);

	async load(): Promise<void> {
		const res = await tasksApi.inbox(getApiClient());
		this.count = res.count;
		this.warnThresholdExceeded = res.warnThresholdExceeded;
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
