import type { Project, ProjectSuggestionRule } from '../api/types';

/** Upper bound on how many projects are offered for a single task title. */
export const MAX_PROJECT_SUGGESTIONS = 3;

export interface MatchProjectSuggestionsOptions {
	/** Project ids never offered (e.g. the one already selected in the dialog). */
	excludeIds?: number[];
	/** Cap on the returned list; defaults to MAX_PROJECT_SUGGESTIONS. */
	limit?: number;
}

/**
 * Collects the projects suggested for a task title.
 *
 * Every rule whose mask occurs in the title contributes its projects; the union
 * is deduped, stripped of completed/unknown/excluded projects, sorted A-Z by
 * title and capped at `limit`. Suggestions are advisory only — nothing is
 * applied automatically, the user picks from the offered chips.
 */
export function matchProjectSuggestions(
	rules: ProjectSuggestionRule[],
	title: string,
	projects: Project[],
	options: MatchProjectSuggestionsOptions = {}
): Project[] {
	const { excludeIds = [], limit = MAX_PROJECT_SUGGESTIONS } = options;
	if (limit <= 0) return [];

	const lowerTitle = title.toLowerCase();
	const matchedIds = new Set<number>();
	for (const rule of rules) {
		if (!rule.mask) continue;
		const caseSensitive = rule.ignoreCase === false;
		const haystack = caseSensitive ? title : lowerTitle;
		const needle = caseSensitive ? rule.mask : rule.mask.toLowerCase();
		if (!haystack.includes(needle)) continue;
		for (const id of rule.projectIds) matchedIds.add(id);
	}
	if (matchedIds.size === 0) return [];

	const excluded = new Set(excludeIds);
	return projects
		.filter((p) => matchedIds.has(p.id) && !excluded.has(p.id) && p.status !== 'completed')
		.toSorted((a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: 'base' }))
		.slice(0, limit);
}
