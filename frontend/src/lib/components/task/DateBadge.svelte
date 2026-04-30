<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/CalendarBlank';
	import { formatDay } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';

	let {
		value,
		hasTime = false,
		overdue = false,
		hideTodayBadge = false,
		hideTomorrowBadge = false
	}: {
		value: string | null;
		hasTime?: boolean;
		overdue?: boolean;
		hideTodayBadge?: boolean;
		hideTomorrowBadge?: boolean;
	} = $props();

	const tz = $derived(configStore.value?.timezone ?? null);
	const dayOnly = $derived(formatDay(value, false, tz));
	const fullText = $derived(formatDay(value, hasTime, tz));
	const isToday = $derived(dayOnly === 'Today');
	const isTomorrow = $derived(dayOnly === 'Tomorrow');
	const shouldHideDay = $derived(
		(hideTodayBadge && isToday) || (hideTomorrowBadge && isTomorrow)
	);
	const displayText = $derived.by(() => {
		if (!fullText) return '';
		if (!shouldHideDay) return fullText;
		if (!hasTime) return '';
		return fullText.slice(dayOnly.length).trim();
	});
</script>

{#if displayText}
	<span
		class="inline-flex items-center gap-1.5 text-xs"
		class:text-destructive={overdue}
		class:font-medium={overdue || isToday}
		class:text-foreground={!overdue && isToday}
		class:text-muted-foreground={!overdue && !isToday}
		title={value ?? ''}
	>
		<CalendarIcon class="size-3.5 shrink-0" />
		<span>{displayText}</span>
	</span>
{/if}
