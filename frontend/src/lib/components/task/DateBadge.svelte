<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/CalendarBlank';
	import { formatDay } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';

	let {
		value,
		hasTime = false,
		overdue = false,
		hideTodayBadge = false,
		hideTomorrowBadge = false,
		completed = false
	}: {
		value: string | null;
		hasTime?: boolean;
		overdue?: boolean;
		hideTodayBadge?: boolean;
		hideTomorrowBadge?: boolean;
		completed?: boolean;
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

	const spanClass = $derived.by(() => {
		const base = 'inline-flex items-center gap-1.5 text-xs';
		if (completed) {
			const hover = isToday
				? ' group-hover/task:text-foreground group-hover/task:font-medium'
				: '';
			return `${base} text-muted-foreground${hover}`;
		}
		if (overdue) return `${base} text-destructive font-medium`;
		if (isToday) return `${base} text-foreground font-medium`;
		return `${base} text-muted-foreground`;
	});
</script>

{#if displayText}
	<span class={spanClass} title={value ?? ''}>
		<CalendarIcon class="size-3.5 shrink-0" />
		<span>{displayText}</span>
	</span>
{/if}
