<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/CalendarBlank';
	import { formatDay } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';

	let {
		value,
		hasTime = false,
		overdue = false
	}: { value: string | null; hasTime?: boolean; overdue?: boolean } = $props();

	const text = $derived(formatDay(value, hasTime, configStore.value?.timezone ?? null));
	const isToday = $derived(text === 'Today');
</script>

{#if text}
	<span
		class="inline-flex items-center gap-1.5 text-xs"
		class:text-destructive={overdue}
		class:font-medium={overdue || isToday}
		class:text-foreground={!overdue && isToday}
		class:text-muted-foreground={!overdue && !isToday}
		title={value ?? ''}
	>
		<CalendarIcon class="size-3.5 shrink-0" />
		<span>{text}</span>
	</span>
{/if}
