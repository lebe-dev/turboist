<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { DayPart, Priority, Task } from '$lib/api/types';
	import { Button } from '$lib/components/ui/button';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { configStore } from '$lib/stores/config.svelte';
	import { dayKeyInTz, dayStartUtcInTz, shiftDayKey, toIsoUtc } from '$lib/utils/format';
	import { PRIORITY_COLOR, PRIORITY_LABEL, PRIORITY_ORDER } from '$lib/utils/priority';
	import {
		copyTaskTitle,
		deleteTask,
		duplicateTask,
		moveToBacklog,
		removeFromBacklog,
		togglePin,
		updateTaskFields,
		type ListMutator
	} from '$lib/utils/taskActions';
	import ArchiveIcon from 'phosphor-svelte/lib/Archive';
	import CalendarBlankIcon from 'phosphor-svelte/lib/CalendarBlank';
	import CopyIcon from 'phosphor-svelte/lib/Copy';
	import CopySimpleIcon from 'phosphor-svelte/lib/CopySimple';
	import DotsThreeIcon from 'phosphor-svelte/lib/DotsThree';
	import FlagIcon from 'phosphor-svelte/lib/Flag';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import PencilIcon from 'phosphor-svelte/lib/Pencil';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import XIcon from 'phosphor-svelte/lib/X';
	import type { Component } from 'svelte';

	let {
		task,
		mutator,
		belongs,
		onEdit
	}: {
		task: Task;
		mutator: ListMutator;
		belongs?: (task: Task) => boolean;
		onEdit?: (task: Task) => void;
	} = $props();

	const tz = $derived(configStore.value?.timezone ?? null);
	const todayKey = $derived(dayKeyInTz(new Date(), tz));
	const dueKey = $derived(task.dueAt ? dayKeyInTz(new Date(task.dueAt), tz) : null);

	const isToday = $derived(dueKey === todayKey);
	const isTomorrow = $derived(dueKey === shiftDayKey(todayKey, 1));
	const inBacklog = $derived(task.planState === 'backlog');

	function setDate(dayKey: string | null) {
		if (dayKey === null) {
			void updateTaskFields(task, mutator, { dueAt: null, dueHasTime: false }, { belongs });
			return;
		}
		const dueAt = toIsoUtc(dayStartUtcInTz(dayKey, tz));
		void updateTaskFields(task, mutator, { dueAt, dueHasTime: false }, { belongs });
	}

	function setDayPart(part: DayPart) {
		if (task.dayPart === part) return;
		void updateTaskFields(task, mutator, { dayPart: part }, { belongs });
	}

	function setPriority(p: Priority) {
		if (task.priority === p) return;
		void updateTaskFields(task, mutator, { priority: p }, { belongs });
	}

	function handleEdit() {
		if (onEdit) {
			onEdit(task);
			return;
		}
		void goto(resolve('/(app)/task/[id]', { id: String(task.id) }));
	}

	const DAY_PARTS: Array<{ part: DayPart; label: string; icon: Component }> = [
		{ part: 'morning', label: 'Morning', icon: SunHorizonIcon as unknown as Component },
		{ part: 'afternoon', label: 'Afternoon', icon: SunIcon as unknown as Component },
		{ part: 'evening', label: 'Evening', icon: MoonIcon as unknown as Component }
	];
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<Button
				{...props}
				size="sm"
				variant="ghost"
				class="size-8 p-0 text-muted-foreground hover:text-foreground"
				aria-label="Task actions"
				onclick={(e: MouseEvent) => {
					e.stopPropagation();
					(props as { onclick?: (e: MouseEvent) => void }).onclick?.(e);
				}}
			>
				<DotsThreeIcon class="size-5" weight="bold" />
			</Button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align="end" class="min-w-[15rem]">
		<DropdownMenu.Item onclick={handleEdit}>
			<PencilIcon class="size-4" /> Edit
		</DropdownMenu.Item>
		<DropdownMenu.Item onclick={() => void copyTaskTitle(task)}>
			<CopyIcon class="size-4" /> Copy
		</DropdownMenu.Item>
		<DropdownMenu.Item onclick={() => void duplicateTask(task, mutator)}>
			<CopySimpleIcon class="size-4" /> Duplicate
		</DropdownMenu.Item>
		<DropdownMenu.Item onclick={() => void togglePin(task, mutator)}>
			<PushPinIcon class="size-4" weight={task.isPinned ? 'fill' : 'regular'} />
			{task.isPinned ? 'Unpin' : 'Pin'}
		</DropdownMenu.Item>
		{#if task.planState === 'backlog'}
			<DropdownMenu.Item onclick={() => void removeFromBacklog(task, mutator, { belongs })}>
				<ArchiveIcon class="size-4" /> Remove from backlog
			</DropdownMenu.Item>
		{:else}
			<DropdownMenu.Item onclick={() => void moveToBacklog(task, mutator, { belongs })}>
				<ArchiveIcon class="size-4" /> To backlog
			</DropdownMenu.Item>
		{/if}

		<DropdownMenu.Separator />

		<div class="px-2 py-1.5">
			<div class="mb-1.5 text-xs font-medium text-muted-foreground">Date</div>
			<div class="flex items-center gap-1">
				<button
					type="button"
					title="Today"
					aria-label="Today"
					aria-pressed={isToday}
					onclick={() => setDate(todayKey)}
					class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
					class:bg-accent={isToday}
				>
					<CalendarBlankIcon class="size-4 text-emerald-500" />
				</button>
				<button
					type="button"
					title="Tomorrow"
					aria-label="Tomorrow"
					aria-pressed={isTomorrow}
					onclick={() => setDate(shiftDayKey(todayKey, 1))}
					class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
					class:bg-accent={isTomorrow}
				>
					<SunIcon class="size-4 text-amber-500" />
				</button>
				<button
					type="button"
					title={inBacklog ? 'Remove from backlog' : 'To backlog'}
					aria-label={inBacklog ? 'Remove from backlog' : 'To backlog'}
					aria-pressed={inBacklog}
					onclick={() =>
						inBacklog
							? void removeFromBacklog(task, mutator, { belongs })
							: void moveToBacklog(task, mutator, { belongs })}
					class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
					class:bg-accent={inBacklog}
				>
					<ArchiveIcon class="size-4 text-violet-500" />
				</button>
				<button
					type="button"
					title="Clear date"
					aria-label="Clear date"
					disabled={task.dueAt === null}
					onclick={() => setDate(null)}
					class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
				>
					<XIcon class="size-4" />
				</button>
			</div>
		</div>

		<div class="px-2 py-1.5">
			<div class="mb-1.5 text-xs font-medium text-muted-foreground">Day part</div>
			<div class="flex items-center gap-1">
				{#each DAY_PARTS as opt (opt.part)}
					{@const Icon = opt.icon}
					{@const active = task.dayPart === opt.part}
					<button
						type="button"
						title={opt.label}
						aria-label={opt.label}
						aria-pressed={active}
						onclick={() => setDayPart(opt.part)}
						class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
						class:bg-accent={active}
					>
						<Icon class="size-4" />
					</button>
				{/each}
				<button
					type="button"
					title="Clear day part"
					aria-label="Clear day part"
					disabled={task.dayPart === 'none'}
					onclick={() => setDayPart('none')}
					class="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
				>
					<XIcon class="size-4" />
				</button>
			</div>
		</div>

		<div class="px-2 py-1.5">
			<div class="mb-1.5 text-xs font-medium text-muted-foreground">Priority</div>
			<div class="flex items-center gap-1">
				{#each PRIORITY_ORDER as p (p)}
					{@const active = task.priority === p}
					<button
						type="button"
						title={PRIORITY_LABEL[p]}
						aria-label={PRIORITY_LABEL[p]}
						aria-pressed={active}
						onclick={() => setPriority(p)}
						class="inline-flex size-8 items-center justify-center rounded-md transition-colors hover:bg-accent hover:text-accent-foreground"
						class:bg-accent={active}
					>
						<FlagIcon
							class={`size-4 ${PRIORITY_COLOR[p]}`}
							weight={p === 'no-priority' ? 'regular' : 'fill'}
						/>
					</button>
				{/each}
			</div>
		</div>

		<DropdownMenu.Separator />

		<DropdownMenu.Item variant="destructive" onclick={() => void deleteTask(task, mutator)}>
			<TrashIcon class="size-4" /> Delete
		</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>
