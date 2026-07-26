import { describe, it, expect } from 'vitest';
import { MAX_PROJECT_SUGGESTIONS, matchProjectSuggestions } from './projectSuggestions';
import type { Project, ProjectSuggestionRule } from '../api/types';

function project(id: number, title: string, status: Project['status'] = 'open'): Project {
	return {
		id,
		contextId: 1,
		title,
		description: '',
		color: '#fff',
		status,
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		labels: [],
		troikiCategory: null,
		createdAt: '',
		updatedAt: ''
	};
}

function rule(
	mask: string,
	projectIds: number[],
	ignoreCase = true
): ProjectSuggestionRule {
	return { mask, projectIds, ignoreCase };
}

const projects = [
	project(1, 'Zebra'),
	project(2, 'Alpha'),
	project(3, 'Mango'),
	project(4, 'Beta'),
	project(5, 'Done', 'completed')
];

describe('matchProjectSuggestions', () => {
	it('returns nothing when no rule matches', () => {
		expect(matchProjectSuggestions([rule('deploy', [1])], 'buy milk', projects)).toEqual([]);
	});

	it('matches case-insensitively by default', () => {
		const got = matchProjectSuggestions([rule('deploy', [1])], 'DEPLOY api', projects);
		expect(got.map((p) => p.title)).toEqual(['Zebra']);
	});

	it('respects ignoreCase=false', () => {
		const rules = [rule('Deploy', [1], false)];
		expect(matchProjectSuggestions(rules, 'deploy api', projects)).toEqual([]);
		expect(matchProjectSuggestions(rules, 'Deploy api', projects).map((p) => p.id)).toEqual([1]);
	});

	it('sorts suggestions A-Z by title', () => {
		const got = matchProjectSuggestions([rule('x', [1, 2, 3])], 'x', projects);
		expect(got.map((p) => p.title)).toEqual(['Alpha', 'Mango', 'Zebra']);
	});

	it('caps the result at MAX_PROJECT_SUGGESTIONS', () => {
		const got = matchProjectSuggestions([rule('x', [1, 2, 3, 4])], 'x', projects);
		expect(got).toHaveLength(MAX_PROJECT_SUGGESTIONS);
		expect(got.map((p) => p.title)).toEqual(['Alpha', 'Beta', 'Mango']);
	});

	it('unions and dedupes projects across matching rules', () => {
		const rules = [rule('buy', [2, 3]), rule('milk', [3, 4])];
		const got = matchProjectSuggestions(rules, 'buy milk', projects);
		expect(got.map((p) => p.title)).toEqual(['Alpha', 'Beta', 'Mango']);
	});

	it('skips completed and unknown projects', () => {
		const got = matchProjectSuggestions([rule('x', [5, 999, 2])], 'x', projects);
		expect(got.map((p) => p.id)).toEqual([2]);
	});

	it('excludes ids listed in excludeIds before capping', () => {
		const got = matchProjectSuggestions([rule('x', [1, 2, 3, 4])], 'x', projects, {
			excludeIds: [2]
		});
		expect(got.map((p) => p.title)).toEqual(['Beta', 'Mango', 'Zebra']);
	});

	it('ignores rules with an empty mask', () => {
		expect(matchProjectSuggestions([rule('', [1])], 'anything', projects)).toEqual([]);
	});
});
