<script lang="ts">
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import type { CalendarEvent, DayPart } from '$lib/api/types';
	import { t } from '$lib/i18n';
	import { eventTimeLabel } from '$lib/utils/calendar';
	import CalendarEventActionsMenu from './CalendarEventActionsMenu.svelte';

	let {
		event,
		timezone = null,
		dayPart = 'none',
		completed = false,
		showActions = true
	}: {
		event: CalendarEvent;
		timezone?: string | null;
		dayPart?: DayPart;
		completed?: boolean;
		showActions?: boolean;
	} = $props();

	const timeLabel = $derived(event.allDay ? '' : eventTimeLabel(event, timezone, $t('calendar.allDay')));
	const hasMeta = $derived(timeLabel.length > 0);

	function openInGoogle(): void {
		if (!event.htmlLink || typeof window === 'undefined') return;
		window.open(event.htmlLink, '_blank', 'noopener,noreferrer');
	}

	function onRowClick(e: MouseEvent): void {
		if ((e.target as HTMLElement | null)?.closest('[data-calendar-marker]')) return;
		openInGoogle();
	}

	function onRowKeydown(e: KeyboardEvent): void {
		if (e.key !== 'Enter' && e.key !== ' ') return;
		e.preventDefault();
		openInGoogle();
	}
</script>

<div
	role="link"
	tabindex="0"
	onclick={onRowClick}
	onkeydown={onRowKeydown}
	aria-disabled={!event.htmlLink}
	class="group/task relative flex gap-3 rounded-lg px-3 transition-colors hover:bg-accent/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
	class:cursor-pointer={!!event.htmlLink}
	class:items-start={hasMeta}
	class:items-center={!hasMeta}
	class:py-2.5={hasMeta}
	class:py-1.5={!hasMeta}
	style:padding-left="0.75rem"
>
	<span
		data-calendar-marker
		class="inline-flex size-4 shrink-0 items-center justify-center rounded-full"
		class:mt-0.5={hasMeta}
		style={`color:${event.sourceColor || '#9ca3af'}`}
		title={$t('calendar.openInGoogle')}
		aria-hidden="true"
	>
		<CalendarIcon class="size-4" weight="bold" />
	</span>

	<div class="flex min-w-0 flex-1 flex-col gap-1">
		<div class="flex items-center gap-2">
			<p
				class="min-w-0 flex-1 break-words text-sm leading-snug md:truncate"
				class:font-medium={!completed}
				class:line-through={completed}
				class:text-muted-foreground={completed}
				class:text-foreground={!completed}
			>
				{event.title}
			</p>
		</div>

		{#if timeLabel}
			<p class="break-words text-xs text-muted-foreground/70 md:truncate">
				<span class="tabular-nums">{timeLabel}</span>
			</p>
		{/if}
	</div>

	{#if showActions}
		<div class="flex items-center self-center">
			<CalendarEventActionsMenu {event} {timezone} {dayPart} />
		</div>
	{/if}
</div>
