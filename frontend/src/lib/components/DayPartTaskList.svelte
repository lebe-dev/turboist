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
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { t } from 'svelte-intl-precompile';

	let {
		tasks,
		dayParts,
		timezone = '',
		view = 'today',
		searchQuery = '',
		contextName = '',
		onResetContext
	}: {
		tasks: Task[];
		dayParts: DayPart[];
		timezone?: string;
		view?: 'today' | 'tomorrow';
		searchQuery?: string;
		contextName?: string;
		onResetContext?: () => void;
	} = $props();

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

		const sortTasks = view === 'tomorrow'
			? (list: Task[]) => [...list].sort((a, b) => b.priority - a.priority)
			: (list: Task[]) => list;

		const result: Section[] = [];
		for (const dp of dayParts) {
			const t = sectionMap.get(dp.label)!;
			result.push({
				key: dp.label,
				label: dp.label,
				timeRange: `${dp.start}:00\u2013${dp.end}:00`,
				dayPart: dp,
				tasks: sortTasks(t)
			});
		}

		const unassigned = sectionMap.get('__unassigned__')!;
		result.push({
			key: '__unassigned__',
			label: $t('tasks.noTime'),
			timeRange: '',
			dayPart: null,
			tasks: sortTasks(unassigned)
		});

		return result;
	});

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
		if (view === 'tomorrow') return false;
		if (section.dayPart === null) return hoveredSection !== section.key;
		return section.dayPart.label !== currentDayPartLabel;
	}

	const dayPartLabels = $derived(new Set(dayParts.map((dp) => dp.label)));

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
		<p class="text-sm">{$t('tasks.noTasks')}</p>
		{#if contextName}
			<p class="mt-2 text-xs text-muted-foreground/60">
				{$t('tasks.context', { values: { name: contextName } })}
				{#if onResetContext}
					<span class="mx-1">·</span>
					<button class="text-muted-foreground/60 underline underline-offset-2 transition-colors hover:text-muted-foreground" onclick={onResetContext}>{$t('tasks.reset')}</button>
				{/if}
			</p>
		{/if}
	</div>
{:else}
	<div class="space-y-4">
		{#each sections as section, sectionIdx (section.key)}
			{@const Icon = sectionIcon(sectionIdx, dayParts.length)}
			{#if section.tasks.length > 0}
				{@const isActive = view !== 'tomorrow' && currentDayPartLabel !== null && section.dayPart?.label === currentDayPartLabel}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="rounded-xl transition-all duration-300 {isActive ? 'border border-border/60 bg-muted/30 py-2.5' : 'py-0'}"
					onmouseenter={() => hoveredSection = section.key}
					onmouseleave={() => hoveredSection = null}
				>
					<div class="mb-1.5 flex items-center gap-2 px-2 md:px-3">
						<Icon class={isActive ? 'h-4 w-4 text-foreground/70' : 'h-3.5 w-3.5 text-muted-foreground/60'} />
						<span class="font-semibold uppercase tracking-wider {isActive ? 'text-[13px] text-foreground/70' : 'text-[11px] text-muted-foreground/60'}">
							{section.label}
						</span>
						{#if section.timeRange}
							<span class="{isActive ? 'text-[11px] text-foreground/40' : 'text-[10px] text-muted-foreground/40'}">{section.timeRange}</span>
						{/if}
						<span class="tabular-nums {isActive ? 'text-[11px] text-foreground/40' : 'text-[10px] text-muted-foreground/40'}">{section.tasks.length}</span>
					</div>
					<div class="space-y-px px-1">
						{#each section.tasks as task, i (task.id)}
							<div class="animate-fade-in-up" style="animation-delay: {Math.min(i * 30, 300)}ms">
								<TaskItem {task} {searchQuery} dimmed={isDimmed(section)} hideTodayDue={view === 'today'} hideTomorrowDue={view === 'tomorrow'}>
									{#snippet dropdownExtra()}
										<div class="px-2 py-1.5">
											<p class="text-xs font-semibold text-muted-foreground">{$t('task.timeOfDay')}</p>
											<div class="mt-1.5 flex items-center gap-1">
												{#each dayParts as dp, dpIdx (dp.label)}
													{@const DPIcon = sectionIcon(dpIdx, dayParts.length)}
													<button
														class="flex h-7 w-7 items-center justify-center rounded-md transition-colors
															{dp.label === section.dayPart?.label ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
														title={dp.label}
														onclick={() => { if (dp.label !== section.dayPart?.label) moveTask(task, dp.label); }}
													>
														<DPIcon class="h-4 w-4" />
													</button>
												{/each}
												{#if section.dayPart}
													<button
														class="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
														title="Remove time"
														onclick={() => moveTask(task, null)}
													>
														<XIcon class="h-3.5 w-3.5" />
													</button>
												{/if}
											</div>
										</div>
									{/snippet}
								</TaskItem>
							</div>
						{/each}
					</div>
				</div>
			{/if}
		{/each}
	</div>
{/if}
