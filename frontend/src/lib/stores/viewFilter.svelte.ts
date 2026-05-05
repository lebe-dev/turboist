class ViewFilterStore {
	title = $state<string | null>(null);

	setTitle(title: string): void {
		this.title = title;
	}

	clear(): void {
		this.title = null;
	}
}

export const viewFilterStore = new ViewFilterStore();
