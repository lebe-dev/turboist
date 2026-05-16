<script lang="ts">
	import CalendarPlusIcon from 'phosphor-svelte/lib/CalendarPlus';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import type { CalendarEvent, DayPart } from '$lib/api/types';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { t } from '$lib/i18n';
	import { dayKeyInTz } from '$lib/utils/format';

	let {
		event,
		timezone = null,
		dayPart = 'none'
	}: {
		event: CalendarEvent;
		timezone?: string | null;
		dayPart?: DayPart;
	} = $props();

	function eventDayKey(): string {
		if (event.allDay && event.startDate) return event.startDate;
		const start = new Date(event.start);
		if (Number.isNaN(start.getTime())) return '';
		return dayKeyInTz(start, timezone);
	}

	function openQuickAddFromEvent(): void {
		window.dispatchEvent(
			new CustomEvent('turboist:quick-add', {
				detail: {
					title: event.title,
					dueDate: eventDayKey(),
					dayPart: event.allDay ? 'none' : dayPart
				}
			})
		);
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<Button
				{...props}
				size="sm"
				variant="ghost"
				class="size-8 p-0 text-muted-foreground hover:text-foreground"
				aria-label={$t('calendar.actionsAriaLabel')}
				onclick={(e: MouseEvent) => {
					e.stopPropagation();
					(props as { onclick?: (e: MouseEvent) => void }).onclick?.(e);
				}}
			>
				<DotsThreeIcon class="size-5" weight="bold" />
			</Button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align="end" class="min-w-[13rem]">
		<DropdownMenu.Item
			onclick={(e: MouseEvent) => {
				e.stopPropagation();
				openQuickAddFromEvent();
			}}
		>
			<CalendarPlusIcon class="size-4" /> {$t('calendar.createTask')}
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>
