<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { DayPart } from '$lib/api/types';
	import type { DayPartInterval } from '$lib/utils/viewGroup';
	import DayPartSectionHeader from './DayPartSectionHeader.svelte';

	let {
		part,
		label,
		interval,
		count,
		active = false,
		children
	}: {
		part: DayPart;
		label: string;
		interval: DayPartInterval | null;
		count: number;
		active?: boolean;
		children: Snippet;
	} = $props();
</script>

<section
	class="rounded-lg border px-1 py-2 transition-colors"
	class:border-border={active}
	class:border-transparent={!active}
>
	<DayPartSectionHeader {part} {label} {interval} {count} />
	{@render children()}
</section>
