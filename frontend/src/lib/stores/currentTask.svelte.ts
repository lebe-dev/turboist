function createCurrentTaskStore() {
	let projectId = $state<number | null>(null);
	let labelIds = $state<number[]>([]);

	return {
		get projectId(): number | null {
			return projectId;
		},
		get labelIds(): number[] {
			return labelIds;
		},
		set(pId: number | null, lIds: number[]): void {
			projectId = pId;
			labelIds = lIds;
		},
		clear(): void {
			projectId = null;
			labelIds = [];
		}
	};
}

export const currentTaskStore = createCurrentTaskStore();
