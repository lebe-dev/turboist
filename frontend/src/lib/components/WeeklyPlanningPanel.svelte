<script lang="ts">
	import { planningStore } from '$lib/stores/planning.svelte';
	import TaskItem from './TaskItem.svelte';
	import WeeklyProgress from './WeeklyProgress.svelte';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import FlagIcon from '@lucide/svelte/icons/flag';
	import CalendarIcon from '@lucide/svelte/icons/calendar';
	import SunIcon from '@lucide/svelte/icons/sun';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import XIcon from '@lucide/svelte/icons/x';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import { t } from 'svelte-intl-precompile';

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

	// Quick date buttons for upcoming week days
	function weekDays(): { label: string; date: string }[] {
		const days: { label: string; date: string }[] = [];
		const d = new Date();
		const dayNames = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
		for (let i = 2; i <= 7; i++) {
			const next = new Date(d);
			next.setDate(next.getDate() + i);
			const dateStr = next.getFullYear() + '-' + String(next.getMonth() + 1).padStart(2, '0') + '-' + String(next.getDate()).padStart(2, '0');
			days.push({ label: dayNames[next.getDay()], date: dateStr });
		}
		return days;
	}

	let dateInputs: Record<string, HTMLInputElement> = {};

	function openDatePicker(taskId: string) {
		requestAnimationFrame(() => {
			dateInputs[taskId]?.showPicker?.();
			dateInputs[taskId]?.focus();
		});
	}

	async function onDatePicked(taskId: string, e: Event) {
		const value = (e.target as HTMLInputElement).value;
		if (value) {
			await planningStore.updateWeeklyTask(taskId, { due_date: value });
		}
	}
</script>

<div class="flex h-full flex-col">
	<div class="shrink-0">
		<WeeklyProgress weekly_count={planningStore.meta.weekly_count} weekly_limit={planningStore.meta.weekly_limit} />
	</div>

	<div class="shrink-0 border-b border-border/50 px-4 py-3">
		<h2 class="text-xs font-semibold uppercase tracking-wider text-muted-foreground/60">
			{$t('planning.thisWeek')}
		</h2>
	</div>

	<div class="flex-1 overflow-y-auto px-1 py-2">
		{#if planningStore.weeklyTasks.length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
				<InboxIcon class="mb-3 h-10 w-10 animate-float opacity-20" />
				<p class="text-sm">{$t('tasks.noTasks')}</p>
			</div>
		{:else}
			<div class="space-y-px px-1">
				{#each planningStore.weeklyTasks as task (task.id)}
					<div>
						<TaskItem {task}>
							{#snippet actionButton()}
								<button
									class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground/50 transition-colors hover:bg-accent hover:text-foreground"
									onclick={() => planningStore.moveToBacklog(task)}
									aria-label={$t('planning.moveToBacklog')}
									title={$t('planning.moveToBacklog')}
								>
									<ArrowLeftIcon class="h-4 w-4" />
								</button>
							{/snippet}
						</TaskItem>
						<!-- Inline priority + date controls -->
						<div class="flex items-center gap-2 px-8 pb-2">
							<!-- Priority buttons -->
							<div class="flex items-center gap-0.5">
								{#each priorityItems as p (p.value)}
									<button
										class="flex h-6 w-6 items-center justify-center rounded transition-colors {p.color}
											{task.priority === p.value ? 'bg-accent' : 'hover:bg-accent/50'}"
										onclick={() => planningStore.updateWeeklyTask(task.id, { priority: p.value })}
										aria-label={p.label}
										title={p.label}
									>
										<FlagIcon class="h-3 w-3" />
									</button>
								{/each}
							</div>

							<div class="mx-1 h-4 w-px bg-border/50"></div>

							<!-- Date buttons -->
							<div class="flex items-center gap-0.5">
								<button
									class="flex h-6 w-6 items-center justify-center rounded text-green-500 transition-colors
										{task.due?.date === todayStr() ? 'bg-accent' : 'hover:bg-accent/50'}"
									onclick={() => planningStore.updateWeeklyTask(task.id, { due_date: todayStr() })}
									aria-label="Today"
									title="Today"
								>
									<CalendarIcon class="h-3 w-3" />
								</button>
								<button
									class="flex h-6 w-6 items-center justify-center rounded text-amber-500 transition-colors
										{task.due?.date === tomorrowStr() ? 'bg-accent' : 'hover:bg-accent/50'}"
									onclick={() => planningStore.updateWeeklyTask(task.id, { due_date: tomorrowStr() })}
									aria-label="Tomorrow"
									title="Tomorrow"
								>
									<SunIcon class="h-3 w-3" />
								</button>
								<div class="relative">
									<button
										class="flex h-6 w-6 items-center justify-center rounded text-purple-400 transition-colors hover:bg-accent/50"
										onclick={() => openDatePicker(task.id)}
										aria-label="Pick date"
										title="Pick date"
									>
										<ArrowRightIcon class="h-3 w-3" />
									</button>
									<input
										bind:this={dateInputs[task.id]}
										type="date"
										value={task.due?.date ?? ''}
										class="pointer-events-none absolute left-0 top-0 h-0 w-0 opacity-0"
										onchange={(e) => onDatePicked(task.id, e)}
									/>
								</div>
								{#if task.due}
									<button
										class="flex h-6 w-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
										onclick={() => planningStore.updateWeeklyTask(task.id, { due_date: '' })}
										aria-label="Clear date"
										title="Clear date"
									>
										<XIcon class="h-3 w-3" />
									</button>
								{/if}
							</div>

							{#if task.due?.date}
								<span class="text-[11px] text-muted-foreground/60">{task.due.date}</span>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
