export function useListMutator<T extends { id: number; parentId?: number | null }>(opts?: {
	onRemove?: () => void;
}) {
	let items = $state<T[]>([]);
	// Monotonic count of LOCAL writes to this list — `mutator.*` plus any direct
	// `items =` assignment (optimistic insert, rollback, reorder). Deliberately a
	// plain counter, not `$state`: it is read from inside a loader's `isValid()`
	// guard, and making it reactive would turn every fetch into a dependency of
	// whatever effect kicked it off.
	//
	// `usePageLoad` snapshots it when a background revalidation starts and discards
	// the response if it advanced — that response is a pre-write snapshot of server
	// truth, so applying it would undo what the user just did. See the epoch guard
	// there for the full story.
	let epoch = 0;

	const mutator = {
		replace(t: T) {
			epoch += 1;
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			epoch += 1;
			items = items.filter((x) => x.id !== id);
			opts?.onRemove?.();
		},
		/**
		 * Drop `id` together with every descendant present in the list. Used where the
		 * server applies the same cascade — deleting a task deletes its whole subtree,
		 * parking or planning a parent takes its subtasks along — so that the view does
		 * not keep rendering orphaned subtask rows until the next full reload.
		 */
		removeSubtree(id: number) {
			epoch += 1;
			// A plain record rather than a Set: this lives inside a `.svelte.ts` module,
			// where a mutable Set is flagged as a missed reactive collection, and the
			// lookup is purely local bookkeeping.
			const doomed: Record<number, true> = { [id]: true };
			// A child can precede its parent in the list, so sweep until nothing new is
			// picked up rather than filtering in a single pass.
			for (let grew = true; grew; ) {
				grew = false;
				for (const x of items) {
					const pid = x.parentId;
					if (pid == null || doomed[x.id] || !doomed[pid]) continue;
					doomed[x.id] = true;
					grew = true;
				}
			}
			const before = items.length;
			items = items.filter((x) => !doomed[x.id]);
			for (let i = items.length; i < before; i += 1) opts?.onRemove?.();
		},
		insertAfter(id: number, t: T) {
			epoch += 1;
			const idx = items.findIndex((x) => x.id === id);
			if (idx === -1) {
				items = [...items, t];
			} else {
				items = [...items.slice(0, idx + 1), t, ...items.slice(idx + 1)];
			}
		},
		add(t: T) {
			epoch += 1;
			if (items.some((x) => x.id === t.id)) return;
			items = [...items, t];
		}
	};

	return {
		get items() {
			return items;
		},
		/**
		 * Local write: advances the epoch, so a background revalidation already in
		 * flight will not overwrite it. Use for anything the user caused.
		 */
		set items(v: T[]) {
			epoch += 1;
			items = v;
		},
		/**
		 * Apply server truth from a loader — the one write that must NOT advance the
		 * epoch, or every fetch would invalidate its own `isValid()` guard (and with
		 * it any follow-up work sharing that guard, such as the calendar fetch the
		 * day views kick off after the task list lands).
		 */
		setFromServer(v: T[]): void {
			items = v;
		},
		get epoch(): number {
			return epoch;
		},
		mutator
	};
}
