<script lang="ts">
	import * as Sheet from '$lib/components/ui/sheet';
	import type { CalendarEvent } from '$lib/api/types';
	import { locale, t } from '$lib/i18n';
	import { eventTimeLabel, sortCalendarEvents } from '$lib/utils/calendar';
	import { dayKeyInTz, dayStartUtcInTz } from '$lib/utils/format';

	let {
		events,
		timezone = null,
		compact = false
	}: {
		events: CalendarEvent[];
		timezone?: string | null;
		compact?: boolean;
	} = $props();

	let detailsOpen = $state(false);
	let orderedEvents = $derived(sortCalendarEvents(events));

	function eventDayKey(event: CalendarEvent): string {
		if (event.allDay && event.startDate) return event.startDate;
		return dayKeyInTz(new Date(event.start), timezone);
	}

	function sheetDay(): Date | null {
		const event = orderedEvents[0];
		if (!event) return null;
		return dayStartUtcInTz(eventDayKey(event), timezone);
	}

	function sheetDayTitle(): string {
		const day = sheetDay();
		if (!day) return '';
		return day.toLocaleDateString($locale || undefined, {
			timeZone: timezone || undefined,
			day: 'numeric',
			month: 'long'
		});
	}

	function sheetWeekday(): string {
		const day = sheetDay();
		if (!day) return '';
		const value = day.toLocaleDateString($locale || undefined, {
			timeZone: timezone || undefined,
			weekday: 'long'
		});
		return value ? value[0].toLocaleUpperCase($locale || undefined) + value.slice(1) : '';
	}

	function openInGoogle(event: CalendarEvent): void {
		if (!event.htmlLink || typeof window === 'undefined') return;
		window.open(event.htmlLink, '_blank', 'noopener,noreferrer');
	}
</script>

{#if events.length > 0}
	<div class={compact ? 'rounded-md bg-muted/30 px-2 py-1' : 'rounded-md bg-muted/30 px-2.5 py-1.5'}>
		{#each orderedEvents as event (event.id)}
			<button
				type="button"
				class="grid w-full grid-cols-[3px_minmax(0,1fr)] items-center gap-2 rounded-sm px-0.5 py-0.5 text-left text-[11px] leading-4 text-muted-foreground transition-colors hover:bg-background/60 hover:text-foreground"
				onclick={() => (detailsOpen = true)}
			>
				<span class="h-3 rounded-full" style={`background:${event.sourceColor || '#9ca3af'}`}></span>
				<span class="min-w-0 truncate">
					<span class="tabular-nums">{eventTimeLabel(event, timezone, $t('calendar.allDay'))}</span>
					<span class="text-foreground/80"> {event.title}</span>
				</span>
			</button>
		{/each}
	</div>

	<Sheet.Root bind:open={detailsOpen}>
		<Sheet.Content
			side="bottom"
			class="max-h-[78vh] overflow-hidden rounded-t-2xl border-border bg-popover p-0"
			showCloseButton={false}
		>
			<div class="mx-auto mt-2 h-1 w-10 rounded-full bg-muted-foreground/20"></div>
			<div class="overflow-y-auto px-6 pb-8 pt-4">
				<Sheet.Title class="text-2xl font-semibold tracking-normal">{sheetDayTitle()}</Sheet.Title>
				<Sheet.Description class="mt-1 text-sm text-muted-foreground">{sheetWeekday()}</Sheet.Description>

				<div class="mt-8 flex flex-col gap-5">
					{#each orderedEvents as event (event.id)}
						<button
							type="button"
							class="grid w-full grid-cols-[5px_minmax(0,1fr)] gap-3 text-left transition-opacity hover:opacity-80"
							onclick={() => openInGoogle(event)}
							disabled={!event.htmlLink}
						>
							<span class="min-h-16 rounded-full" style={`background:${event.sourceColor || '#9ca3af'}`}></span>
							<span class="min-w-0 py-0.5">
								<span class="block truncate text-xl font-medium leading-7 text-foreground">{event.title}</span>
								<span class="mt-0.5 block text-base text-muted-foreground">
									{eventTimeLabel(event, timezone, $t('calendar.allDay'))}
								</span>
							</span>
						</button>
					{/each}
				</div>
			</div>
		</Sheet.Content>
	</Sheet.Root>
{/if}
