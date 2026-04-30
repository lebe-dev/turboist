export function useListMutator<T extends { id: number }>(opts?: { onRemove?: () => void }) {
	let items = $state<T[]>([]);

	const mutator = {
		replace(t: T) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
			opts?.onRemove?.();
		},
		insertAfter(id: number, t: T) {
			const idx = items.findIndex((x) => x.id === id);
			if (idx === -1) {
				items = [...items, t];
			} else {
				items = [...items.slice(0, idx + 1), t, ...items.slice(idx + 1)];
			}
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
