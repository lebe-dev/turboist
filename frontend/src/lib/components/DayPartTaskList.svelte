<script lang="ts">
	import type { DayPart, Task } from '$lib/api/types';
	import { updateTask } from '$lib/api/client';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import TaskItem from './TaskItem.svelte';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import SunriseIcon from '@lucide/svelte/icons/sunrise';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import ClockIcon from '@lucide/svelte/icons/clock';
	import XIcon from '@lucide/svelte/icons/x';

	let {
		tasks,
		dayParts,
		timezone = '',
		searchQuery = '',
		onselect
	}: {
		tasks: Task[];
		dayParts: DayPart[];
		timezone?: string;
		searchQuery?: string;
		onselect?: (id: string) => void;
	} = $props();

	const dayPartLabels = $derived(new Set(dayParts.map((dp) => dp.label)));

	interface Section {
		key: string;
		label: string;
		timeRange: string;
		dayPart: DayPart | null;
		tasks: Task[];
	}

	const sections = $derived.by(() => {
		const labelToDP = new Map<string, DayPart>();
		for (const dp of dayParts) {
			labelToDP.set(dp.label, dp);
		}

		const sectionMap = new Map<string, Task[]>();
		for (const dp of dayParts) {
			sectionMap.set(dp.label, []);
		}
		sectionMap.set('__unassigned__', []);

		for (const task of tasks) {
			let assigned = false;
			for (const dp of dayParts) {
				if (task.labels.includes(dp.label)) {
					sectionMap.get(dp.label)!.push(task);
					assigned = true;
					break;
				}
			}
			if (!assigned) {
				sectionMap.get('__unassigned__')!.push(task);
			}
		}

		const result: Section[] = [];
		for (const dp of dayParts) {
			const t = sectionMap.get(dp.label)!;
			result.push({
				key: dp.label,
				label: dp.label,
				timeRange: `${dp.start}:00\u2013${dp.end}:00`,
				dayPart: dp,
				tasks: t
			});
		}

		const unassigned = sectionMap.get('__unassigned__')!;
		result.push({
			key: '__unassigned__',
			label: 'Без времени',
			timeRange: '',
			dayPart: null,
			tasks: unassigned
		});

		return result;
	});

	function stripDayPartLabels(task: Task): Task {
		return {
			...task,
			labels: task.labels.filter((l) => !dayPartLabels.has(l)),
			children: task.children.map(stripDayPartLabels)
		};
	}

	function getHourInTimezone(tz: string): number {
		if (!tz) return new Date().getHours();
		return parseInt(new Intl.DateTimeFormat('en-US', { timeZone: tz, hour: 'numeric', hour12: false }).format(new Date()));
	}

	const currentDayPartLabel = $derived.by(() => {
		const hour = getHourInTimezone(timezone);
		return dayParts.find((dp) => hour >= dp.start && hour < dp.end)?.label ?? null;
	});

	function sectionIcon(index: number, total: number) {
		if (index >= total) return ClockIcon; // unassigned
		if (index === 0) return SunriseIcon;
		if (index === total - 1) return MoonIcon;
		return SunIcon;
	}

	let hoveredSection = $state<string | null>(null);

	function isDimmed(section: Section): boolean {
		if (section.dayPart === null) {
			return hoveredSection !== section.key;
		}
		return section.dayPart.label !== currentDayPartLabel;
	}

	function moveTask(task: Task, targetLabel: string | null) {
		const newLabels = task.labels.filter((l) => !dayPartLabels.has(l));
		if (targetLabel) {
			newLabels.push(targetLabel);
		}

		// Optimistic update
		tasksStore.updateTaskLocal(task.id, (t) => ({ ...t, labels: newLabels }));

		// Fire API call in background, refresh on error
		updateTask(task.id, { labels: newLabels }).catch((e) => {
			console.error('Failed to move task', e);
			tasksStore.refresh();
		});
	}
</script>

{#if tasks.length === 0}
	<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
		<InboxIcon class="mb-3 h-10 w-10 animate-float opacity-20" />
		<p class="text-sm">Нет задач</p>
	</div>
{:else}
	<div class="space-y-4">
		{#each sections as section, sectionIdx (section.key)}
			{@const Icon = sectionIcon(sectionIdx, dayParts.length)}
			{#if section.tasks.length > 0}
				{@const isActive = currentDayPartLabel !== null && section.dayPart?.label === currentDayPartLabel}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					onmouseenter={() => hoveredSection = section.key}
					onmouseleave={() => hoveredSection = null}
				>
					<div class="mb-1.5 flex items-center gap-2 px-3">
						<Icon class="h-3.5 w-3.5 {isActive ? 'text-foreground/70' : 'text-muted-foreground/60'}" />
						<span class="text-[11px] font-semibold uppercase tracking-wider {isActive ? 'text-foreground/70' : 'text-muted-foreground/60'}">
							{section.label}
						</span>
						{#if section.timeRange}
							<span class="text-[10px] {isActive ? 'text-foreground/40' : 'text-muted-foreground/40'}">{section.timeRange}</span>
						{/if}
						<span class="text-[10px] tabular-nums {isActive ? 'text-foreground/40' : 'text-muted-foreground/40'}">{section.tasks.length}</span>
					</div>
					<div class="space-y-px px-1">
						{#each section.tasks as task, i (task.id)}
							<div class="animate-fade-in-up group/daypart relative" style="animation-delay: {Math.min(i * 30, 300)}ms">
								<TaskItem task={stripDayPartLabels(task)} {searchQuery} {onselect} dimmed={isDimmed(section)} hideTodayDue />
								<!-- Move buttons -->
								<div
									class="absolute right-2 top-2 flex items-center gap-0.5 rounded-md border border-border/50 bg-popover/95 px-0.5 py-0.5 shadow-sm opacity-0 transition-opacity group-hover/daypart:opacity-100"
								>
									{#each dayParts as dp, dpIdx (dp.label)}
										{@const DPIcon = sectionIcon(dpIdx, dayParts.length)}
										{#if dp.label !== section.dayPart?.label}
											<button
												class="flex h-6 w-6 items-center justify-center rounded text-muted-foreground/60 transition-colors hover:bg-accent hover:text-foreground"
												title={dp.label}
												onclick={() => moveTask(task, dp.label)}
											>
												<DPIcon class="h-3 w-3" />
											</button>
										{/if}
									{/each}
									{#if section.dayPart}
										<button
											class="flex h-6 w-6 items-center justify-center rounded text-muted-foreground/60 transition-colors hover:bg-accent hover:text-foreground"
											title="Remove time"
											onclick={() => moveTask(task, null)}
										>
											<XIcon class="h-3 w-3" />
										</button>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{/each}
	</div>
{/if}
