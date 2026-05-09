import { SvelteSet } from 'svelte/reactivity';

function createTaskSelectionStore() {
	let mode = $state(false);
	let lastClickedId = $state<number | null>(null);
	const ids = new SvelteSet<number>();

	return {
		get mode() {
			return mode;
		},
		get ids() {
			return ids;
		},
		get count() {
			return ids.size;
		},
		get lastClickedId() {
			return lastClickedId;
		},
		enable(): void {
			mode = true;
		},
		disable(): void {
			mode = false;
			lastClickedId = null;
			ids.clear();
		},
		toggle(id: number): void {
			if (ids.has(id)) ids.delete(id);
			else ids.add(id);
			lastClickedId = id;
		},
		add(id: number): void {
			ids.add(id);
			lastClickedId = id;
		},
		clear(): void {
			ids.clear();
			lastClickedId = null;
		},
		has(id: number): boolean {
			return ids.has(id);
		},
		// Selects every id between (inclusive) the last clicked id and `toId` in
		// `visibleIds` order. If no last click is recorded, falls back to a single
		// toggle so shift-clicking the very first checkbox still works.
		selectRange(visibleIds: number[], toId: number): void {
			if (lastClickedId === null) {
				this.toggle(toId);
				return;
			}
			const fromIdx = visibleIds.indexOf(lastClickedId);
			const toIdx = visibleIds.indexOf(toId);
			if (fromIdx === -1 || toIdx === -1) {
				this.toggle(toId);
				return;
			}
			const [lo, hi] = fromIdx <= toIdx ? [fromIdx, toIdx] : [toIdx, fromIdx];
			for (let i = lo; i <= hi; i++) ids.add(visibleIds[i]);
			lastClickedId = toId;
		}
	};
}

export const taskSelectionStore = createTaskSelectionStore();
