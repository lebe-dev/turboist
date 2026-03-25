import type { AutoTagMapping } from '$lib/api/types';

interface CompiledAutoTag {
	label: string;
	mask: string; // normalized: lowercased when ignoreCase=true
	ignoreCase: boolean;
}

export function compileAutoTags(mappings: AutoTagMapping[]): CompiledAutoTag[] {
	return mappings.map((m) => ({
		label: m.label,
		mask: m.ignore_case ? m.mask.toLowerCase() : m.mask,
		ignoreCase: m.ignore_case
	}));
}

export function matchAutoTags(title: string, compiled: CompiledAutoTag[]): string[] {
	return compiled
		.filter((t) => {
			const haystack = t.ignoreCase ? title.toLowerCase() : title;
			return haystack.includes(t.mask);
		})
		.map((t) => t.label);
}
