<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import { formatDay } from '$lib/utils/format';

	let {
		value,
		hasTime = false,
		overdue = false
	}: { value: string | null; hasTime?: boolean; overdue?: boolean } = $props();

	const text = $derived(formatDay(value, hasTime));
</script>

{#if text}
	<span
		class="inline-flex items-center gap-1 text-xs"
		class:text-destructive={overdue}
		class:text-muted-foreground={!overdue}
		title={value ?? ''}
	>
		<CalendarIcon class="size-3" />
		{text}
	</span>
{/if}
