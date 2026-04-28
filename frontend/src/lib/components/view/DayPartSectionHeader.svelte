<script lang="ts">
	import type { DayPart } from '$lib/api/types';
	import type { DayPartInterval } from '$lib/utils/viewGroup';
	import { iconFor } from './dayPartIcon';

	let {
		part,
		label,
		interval,
		count
	}: {
		part: DayPart;
		label: string;
		interval: DayPartInterval | null;
		count: number;
	} = $props();

	const Icon = $derived(iconFor(part));

	function fmtHour(h: number): string {
		return `${h}:00`;
	}
</script>

<div class="flex items-center gap-2 px-3 pb-1 text-xs font-semibold uppercase tracking-wide">
	<Icon class="size-3.5 text-muted-foreground" />
	<span class="text-muted-foreground">{label}</span>
	{#if interval}
		<span class="font-normal normal-case text-muted-foreground/70">
			{fmtHour(interval.start)}–{fmtHour(interval.end)}
		</span>
	{/if}
	<span class="font-normal text-muted-foreground/70">{count}</span>
</div>
