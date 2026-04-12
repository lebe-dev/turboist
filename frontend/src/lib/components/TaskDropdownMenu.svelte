<script lang="ts">
	import type { Task } from '$lib/api/types';
	import type { Snippet } from 'svelte';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Calendar } from '$lib/components/ui/calendar';
	import { type DateValue } from '@internationalized/date';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import SunIcon from '@lucide/svelte/icons/sun';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import XIcon from '@lucide/svelte/icons/x';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import TrashIcon from '@lucide/svelte/icons/trash-2';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import CopyPlusIcon from '@lucide/svelte/icons/copy-plus';
	import ListTreeIcon from '@lucide/svelte/icons/list-tree';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import PinIcon from '@lucide/svelte/icons/pin';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import TagIcon from '@lucide/svelte/icons/tag';
	import { t } from 'svelte-intl-precompile';

	let {
		open = $bindable(false),
		onOpenChange,
		trigger,
		task,

		onEdit,
		onDuplicate,
		onCopy,
		onDecompose,

		canPin = false,
		isPinned = false,
		onPin,

		backlogLabel = '',
		isInBacklog = false,
		onToggleBacklog,

		subtaskCount = 0,
		onResetSubtaskPriorities,
		onResetSubtaskLabels,
		onBulkTodayToday,
		onBulkTodayTomorrow,

		dropdownExtra,

		onSetDate,
		onClearDate,
		onOpenDatePicker,
		showCalendar = false,
		calendarValue,
		onCalendarSelect,

		onSetPriority,

		onDelete,

		hideDecompose = false,
		hidePriority = false,

		labelBlocked = false,
		priorityBlocked = false,
		labelBlockedTooltip = '',
		postponeExhausted = false,

		width = 'w-52',
		align = 'end'
	}: {
		open?: boolean;
		onOpenChange?: (open: boolean) => void;
		trigger?: Snippet;
		task: Task;

		onEdit?: () => void;
		onDuplicate?: () => void;
		onCopy?: () => void;
		onDecompose?: () => void;

		canPin?: boolean;
		isPinned?: boolean;
		onPin?: () => void;

		backlogLabel?: string;
		isInBacklog?: boolean;
		onToggleBacklog?: () => void;

		subtaskCount?: number;
		onResetSubtaskPriorities?: () => void;
		onResetSubtaskLabels?: () => void;
		onBulkTodayToday?: () => void;
		onBulkTodayTomorrow?: () => void;

		dropdownExtra?: Snippet;

		onSetDate: (date: string) => void;
		onClearDate?: () => void;
		onOpenDatePicker?: () => void;
		showCalendar?: boolean;
		calendarValue?: DateValue;
		onCalendarSelect?: (v: DateValue | undefined) => void;

		onSetPriority: (p: number) => void;

		onDelete: () => void;

		hideDecompose?: boolean;
		hidePriority?: boolean;

		labelBlocked?: boolean;
		priorityBlocked?: boolean;
		labelBlockedTooltip?: string;
		postponeExhausted?: boolean;

		width?: string;
		align?: 'start' | 'end' | 'center';
	} = $props();

	const priorityItems = [
		{ value: 4, label: 'P1', color: 'text-red-500' },
		{ value: 3, label: 'P2', color: 'text-amber-500' },
		{ value: 2, label: 'P3', color: 'text-blue-400' },
		{ value: 1, label: 'P4', color: 'text-muted-foreground' }
	];

	function todayStr(): string {
		const d = new Date();
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	function tomorrowStr(): string {
		const d = new Date();
		d.setDate(d.getDate() + 1);
		return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
	}

	function handleOpenChange(v: boolean) {
		open = v;
		onOpenChange?.(v);
	}
</script>

<DropdownMenu.Root {open} onOpenChange={handleOpenChange}>
	{@render trigger?.()}
	<DropdownMenu.Content {align} class={showCalendar ? 'w-72' : width}>
		{#if onEdit}
			<DropdownMenu.Item onclick={onEdit}>
				<PencilIcon class="h-4 w-4" />
				{$t('task.edit')}
			</DropdownMenu.Item>
		{/if}

		{#if onDuplicate}
			<DropdownMenu.Item onclick={onDuplicate}>
				<CopyPlusIcon class="h-4 w-4" />
				{$t('task.duplicate')}
			</DropdownMenu.Item>
		{/if}

		{#if onCopy}
			<DropdownMenu.Item onclick={onCopy}>
				<CopyIcon class="h-4 w-4" />
				{$t('task.copy')}
			</DropdownMenu.Item>
		{/if}

		{#if onDecompose && !hideDecompose}
			<DropdownMenu.Item onclick={() => { handleOpenChange(false); onDecompose(); }}>
				<ListTreeIcon class="h-4 w-4" />
				{$t('task.decompose')}
			</DropdownMenu.Item>
		{/if}

		{#if canPin && onPin}
			<DropdownMenu.Item onclick={onPin}>
				<PinIcon class="h-4 w-4" />
				{isPinned ? $t('task.unpin') : $t('task.pin')}
			</DropdownMenu.Item>
		{/if}

		{#if backlogLabel && onToggleBacklog}
			<DropdownMenu.Item onclick={onToggleBacklog}>
				<InboxIcon class="h-4 w-4" />
				{isInBacklog ? $t('task.removeFromBacklog') : $t('task.addToBacklog')}
			</DropdownMenu.Item>
		{/if}

		{#if subtaskCount > 0 && (onResetSubtaskPriorities || onResetSubtaskLabels || onBulkTodayToday || onBulkTodayTomorrow)}
			<DropdownMenu.Sub>
				<DropdownMenu.SubTrigger>
					<LayersIcon class="h-4 w-4" />
					{$t('task.bulkOperations')}
				</DropdownMenu.SubTrigger>
				<DropdownMenu.SubContent>
					{#if onResetSubtaskPriorities}
						<DropdownMenu.Item onclick={onResetSubtaskPriorities}>
							<FlagIcon class="h-4 w-4" />
							{$t('task.resetSubtaskPriorities')}
						</DropdownMenu.Item>
					{/if}
					{#if onResetSubtaskLabels}
						<DropdownMenu.Item onclick={onResetSubtaskLabels}>
							<TagIcon class="h-4 w-4" />
							{$t('task.resetSubtaskLabels')}
						</DropdownMenu.Item>
					{/if}
					{#if onBulkTodayToday || onBulkTodayTomorrow}
						<DropdownMenu.Separator />
					{/if}
					{#if onBulkTodayToday}
						<DropdownMenu.Item onclick={onBulkTodayToday}>
							<CalendarIcon class="h-4 w-4 text-green-500" />
							{$t('task.bulkTodayToday')}
						</DropdownMenu.Item>
					{/if}
					{#if onBulkTodayTomorrow}
						<DropdownMenu.Item onclick={onBulkTodayTomorrow}>
							<SunIcon class="h-4 w-4 text-amber-500" />
							{$t('task.bulkTodayTomorrow')}
						</DropdownMenu.Item>
					{/if}
				</DropdownMenu.SubContent>
			</DropdownMenu.Sub>
		{/if}

		<DropdownMenu.Separator />

		<!-- Date section -->
		<div class="px-2 py-1.5">
			<p class="text-xs font-semibold text-muted-foreground">{$t('task.date')}</p>
			<div class="mt-1.5 flex items-center gap-1">
				<button
					class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
						{labelBlocked || priorityBlocked
						? 'text-muted-foreground/40 cursor-not-allowed'
						: task.due?.date === todayStr() ? 'bg-accent text-green-500' : 'text-green-500 hover:bg-accent'}"
					onclick={() => { if (!labelBlocked && !priorityBlocked) { onSetDate(todayStr()); handleOpenChange(false); } }}
					disabled={labelBlocked || priorityBlocked}
					title={labelBlocked ? labelBlockedTooltip : priorityBlocked ? $t('constraints.priorityFloor') : undefined}
					aria-label="Today"
				>
					<CalendarIcon class="h-4 w-4" />
				</button>
				<button
					class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
						{labelBlocked || postponeExhausted
						? 'text-muted-foreground/40 cursor-not-allowed'
						: task.due?.date === tomorrowStr() ? 'bg-accent text-amber-500' : 'text-amber-500 hover:bg-accent'}"
					onclick={() => { if (!labelBlocked && !postponeExhausted) { onSetDate(tomorrowStr()); handleOpenChange(false); } }}
					disabled={labelBlocked || postponeExhausted}
					title={labelBlocked ? labelBlockedTooltip : postponeExhausted ? $t('constraints.postponeLimitReached') : undefined}
					aria-label="Tomorrow"
				>
					<SunIcon class="h-4 w-4" />
				</button>
				{#if onOpenDatePicker}
					<button
						class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
							{labelBlocked || postponeExhausted ? 'text-muted-foreground/40 cursor-not-allowed' : 'text-purple-400 hover:bg-accent'}"
						onclick={() => { if (!labelBlocked && !postponeExhausted) onOpenDatePicker?.(); }}
						disabled={labelBlocked || postponeExhausted}
						title={labelBlocked ? labelBlockedTooltip : postponeExhausted ? $t('constraints.postponeLimitReached') : undefined}
						aria-label="Pick date"
					>
						<ArrowRightIcon class="h-4 w-4" />
					</button>
				{/if}
				{#if task.due && onClearDate}
					<button
						class="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
						onclick={() => { onClearDate?.(); handleOpenChange(false); }}
						aria-label="Clear date"
					>
						<XIcon class="h-3.5 w-3.5" />
					</button>
				{/if}
			</div>
			{#if showCalendar && onCalendarSelect}
				<div class="mt-1">
					<Calendar
						type="single"
						value={calendarValue}
						onValueChange={onCalendarSelect}
						class="rounded-md border border-border"
					/>
				</div>
			{/if}
		</div>

		{#if dropdownExtra}
			{@render dropdownExtra()}
		{/if}

		<!-- Priority section -->
		{#if !hidePriority}
			<div class="px-2 py-1.5">
				<p class="text-xs font-semibold text-muted-foreground">{$t('task.priority')}</p>
				<div class="mt-1.5 flex items-center gap-1">
					{#each priorityItems as p (p.value)}
						<button
							class="flex h-7 w-7 items-center justify-center rounded-md transition-colors {p.color}
								{task.priority === p.value ? 'bg-accent' : 'hover:bg-accent'}"
							onclick={() => { onSetPriority(p.value); handleOpenChange(false); }}
							aria-label={p.label}
						>
							<FlagIcon class="h-4 w-4" />
						</button>
					{/each}
				</div>
			</div>
		{/if}

		<DropdownMenu.Separator />

		<DropdownMenu.Item variant="destructive" onclick={onDelete}>
			<TrashIcon class="h-4 w-4" />
			{$t('dialog.delete')}
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>
