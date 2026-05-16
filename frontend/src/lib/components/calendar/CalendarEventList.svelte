<script lang="ts">
	import ArrowSquareOutIcon from 'phosphor-svelte/lib/ArrowSquareOut';
	import CalendarBlankIcon from 'phosphor-svelte/lib/CalendarBlank';
	import MapPinIcon from 'phosphor-svelte/lib/MapPin';
	import type { CalendarEvent } from '$lib/api/types';
	import { t } from '$lib/i18n';
	import { eventTimeLabel } from '$lib/utils/calendar';

	let {
		events,
		timezone = null,
		compact = false
	}: {
		events: CalendarEvent[];
		timezone?: string | null;
		compact?: boolean;
	} = $props();
</script>

{#if events.length > 0}
	<section class="rounded-md border border-border/50 bg-muted/20 px-3 py-2">
		<header class="mb-1.5 flex items-center gap-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
			<CalendarBlankIcon class="size-3.5" />
			<span>{$t('calendar.title')}</span>
		</header>
		<div class="flex flex-col gap-1">
			{#each events as event (event.id)}
				<div
					class="grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-x-2 rounded px-1 py-1 text-xs transition-colors hover:bg-background/70"
				>
					<span class="mt-[0.2rem] h-2 w-2 rounded-full" style={`background:${event.sourceColor || '#9ca3af'}`}></span>
					<span class="min-w-0">
						<span class="flex min-w-0 items-baseline gap-2">
							<span class="shrink-0 tabular-nums text-muted-foreground">{eventTimeLabel(event, timezone, $t('calendar.allDay'))}</span>
							<span class="truncate text-foreground/90">{event.title}</span>
						</span>
						{#if !compact}
							<span class="mt-0.5 flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground">
								<span class="truncate">{event.sourceName}</span>
								{#if event.location}
									<span class="inline-flex min-w-0 items-center gap-1">
										<MapPinIcon class="size-3" />
										<span class="truncate">{event.location}</span>
									</span>
								{/if}
							</span>
						{/if}
					</span>
					{#if event.htmlLink}
						<a
							href={event.htmlLink}
							target="_blank"
							rel="noreferrer"
							aria-label={$t('calendar.openInGoogle')}
							title={$t('calendar.openInGoogle')}
							class="rounded p-1 text-muted-foreground transition-colors hover:bg-background hover:text-foreground"
						>
							<ArrowSquareOutIcon class="size-3.5" />
						</a>
					{/if}
				</div>
			{/each}
		</div>
	</section>
{/if}
