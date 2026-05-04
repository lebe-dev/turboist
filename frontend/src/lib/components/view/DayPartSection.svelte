<script lang="ts">
	import { setContext } from 'svelte';
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
		onBulkMove,
		children
	}: {
		part: DayPart;
		label: string;
		interval: DayPartInterval | null;
		count: number;
		active?: boolean;
		onBulkMove?: (targetPart: DayPart) => void;
		children: Snippet;
	} = $props();

	setContext('dayPartActive', () => active);
</script>

<section
	class="rounded-lg border px-1 py-2 transition-colors"
	class:border-border={active}
	class:border-transparent={!active}
>
	<DayPartSectionHeader {part} {label} {interval} {count} {active} {onBulkMove} />
	{@render children()}
</section>
