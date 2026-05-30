// reconcileByVersion merges a freshly fetched list into the current one while
// preserving object identity for entries that have not changed. Two entries are
// considered identical when they share the same `id` and `updatedAt` — the
// app's Last-Write-Wins version marker. Reusing the old reference lets Svelte's
// keyed `{#each}` skip re-rendering rows that did not actually change during a
// background revalidation (SSE catch-up, reconnect sweep), which is the main
// source of project-page re-render churn.
//
// The result mirrors `incoming` exactly in order and membership — only the
// object references differ. When nothing changed (same length, same order, every
// entry reusable) the original `current` array reference is returned so derived
// state downstream does not recompute at all.
export function reconcileByVersion<T extends { id: number; updatedAt: string }>(
	current: readonly T[],
	incoming: readonly T[]
): T[] {
	if (current.length === 0) return incoming as T[];

	const byId = new Map<number, T>();
	for (const item of current) byId.set(item.id, item);

	let changed = incoming.length !== current.length;
	const result = incoming.map((next, i) => {
		const prev = byId.get(next.id);
		if (prev && prev.updatedAt === next.updatedAt) {
			if (prev !== current[i]) changed = true; // same item, different position
			return prev;
		}
		changed = true;
		return next;
	});

	return changed ? result : (current as T[]);
}
