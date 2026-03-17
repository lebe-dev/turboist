function createLabelFilterStore() {
	let activeLabel = $state<string | null>(null);

	return {
		get activeLabel() {
			return activeLabel;
		},
		set(label: string) {
			activeLabel = label;
		},
		clear() {
			activeLabel = null;
		}
	};
}

export const labelFilterStore = createLabelFilterStore();
