<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import EmptyState from '$lib/components/view/EmptyState.svelte';
	import DayPartSection from '$lib/components/view/DayPartSection.svelte';
	import CompletedTodayFooter from '$lib/components/view/CompletedTodayFooter.svelte';
	import { activeDayPart, groupByDayPart } from '$lib/utils/viewGroup';
	import { parseIso, dayKeyInTz } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';
	import {
		toggleComplete,
		togglePin,
		deleteTask,
		describeError
	} from '$lib/utils/taskActions';

	let items = $state<Task[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let completedCount = $state(0);

	const dayParts = $derived(configStore.value?.dayParts);
	const tz = $derived(configStore.value?.timezone ?? null);
	const groups = $derived(groupByDayPart(items, dayParts));
	const active = $derived(activeDayPart(new Date(), dayParts, tz));

	const mutator = {
		replace(t: Task) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
			total = Math.max(0, total - 1);
			completedCount += 1;
		}
	};

	async function load(): Promise<void> {
		loading = true;
		try {
			const [open, completed] = await Promise.all([
				viewsApi.today(getApiClient()),
				viewsApi.completedToday(getApiClient(), { limit: 1 })
			]);
			items = open.items;
			total = open.total;
			completedCount = completed.total;
		} catch (err) {
			toast.error(describeError(err, 'Failed to load today'));
		} finally {
			loading = false;
		}
	}

	function isToday(t: Task): boolean {
		const dt = parseIso(t.dueAt);
		if (!dt) return false;
		return dayKeyInTz(dt, tz) === dayKeyInTz(new Date(), tz);
	}

	function onUncompletedFromFooter(): void {
		completedCount = Math.max(0, completedCount - 1);
	}

	onMount(load);
</script>

<ViewHeader title="Today" subtitle={loading ? 'Loading…' : `${total} task${total === 1 ? '' : 's'}`} />

<div class="px-2 py-2">
	{#if loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else if items.length === 0 && completedCount === 0}
		<EmptyState
			icon={SunIcon}
			title="Nothing for today"
			description="No tasks are scheduled for today. Enjoy the calm."
		/>
	{:else}
		<div class="flex flex-col gap-4 py-2">
			{#each groups as group (group.part)}
				<DayPartSection
					part={group.part}
					label={group.label}
					interval={group.interval}
					count={group.tasks.length}
					active={group.part === active}
				>
					<TaskTree
						tasks={group.tasks}
						hideDayPart
						onToggle={(t) => toggleComplete(t, mutator, { belongs: isToday })}
						onPinToggle={(t) => togglePin(t, mutator)}
						onDelete={(t) => deleteTask(t, mutator)}
					/>
				</DayPartSection>
			{/each}

			<CompletedTodayFooter count={completedCount} onUncompleteOutside={onUncompletedFromFooter} />
		</div>
	{/if}
</div>
