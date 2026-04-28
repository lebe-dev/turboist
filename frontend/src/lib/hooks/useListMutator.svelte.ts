export function useListMutator<T extends { id: number }>(opts?: { onRemove?: () => void }) {
	let items = $state<T[]>([]);

	const mutator = {
		replace(t: T) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
			opts?.onRemove?.();
		}
	};

	return {
		get items() {
			return items;
		},
		set items(v: T[]) {
			items = v;
		},
		mutator
	};
}
