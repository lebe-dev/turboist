import type { Task } from '$lib/api/types';

// Merge upserted tasks into an existing tree by ID (replace or append at root level).
export function mergeUpserted(existing: Task[], upserted: Task[]): Task[] {
	if (upserted.length === 0) return existing;

	const upsertedById = new Map<string, Task>();
	for (const t of upserted) {
		upsertedById.set(t.id, t);
	}

	const result: Task[] = [];
	const seen = new Set<string>();

	// Replace existing entries
	for (const t of existing) {
		const replacement = upsertedById.get(t.id);
		if (replacement) {
			result.push(replacement);
			seen.add(t.id);
		} else {
			result.push(t);
		}
	}

	// Append new entries that weren't replacements
	for (const t of upserted) {
		if (!seen.has(t.id)) {
			result.push(t);
		}
	}

	return result;
}

// Remove tasks by IDs from a tree (recursive through children).
export function filterByIds(tasks: Task[], removeIds: string[]): Task[] {
	if (removeIds.length === 0) return tasks;

	const idSet = new Set(removeIds);

	function walk(list: Task[]): Task[] {
		return list.flatMap((t) => {
			if (idSet.has(t.id)) return [];
			return [{ ...t, children: walk(t.children) }];
		});
	}

	return walk(tasks);
}
