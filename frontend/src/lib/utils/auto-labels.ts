import type { AutoLabelMapping } from '$lib/api/types';

interface CompiledAutoLabel {
	label: string;
	mask: string; // normalized: lowercased when ignoreCase=true
	ignoreCase: boolean;
}

export function compileAutoLabels(mappings: AutoLabelMapping[]): CompiledAutoLabel[] {
	return mappings.map((m) => ({
		label: m.label,
		mask: m.ignore_case ? m.mask.toLowerCase() : m.mask,
		ignoreCase: m.ignore_case
	}));
}

export function matchAutoLabels(title: string, compiled: CompiledAutoLabel[]): string[] {
	return compiled
		.filter((t) => {
			const haystack = t.ignoreCase ? title.toLowerCase() : title;
			return haystack.includes(t.mask);
		})
		.map((t) => t.label);
}
