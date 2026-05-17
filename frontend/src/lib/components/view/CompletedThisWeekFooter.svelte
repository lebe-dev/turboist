<script lang="ts">
	import { toast } from 'svelte-sonner';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import GroupHeader from '$lib/components/view/GroupHeader.svelte';
	import { groupByCompletedDay } from '$lib/utils/viewGroup';
	import { describeError, toggleComplete } from '$lib/utils/taskActions';
	import { t } from '$lib/i18n';

	let expanded = $state(false);
	let loaded = $state(false);
	let loading = $state(false);
	let items = $state<Task[]>([]);

	const tz = $derived(configStore.value?.timezone ?? null);
	const groups = $derived(groupByCompletedDay(items, tz));

	async function ensureLoaded(): Promise<void> {
		if (loaded || loading) return;
		loading = true;
		try {
			const res = await viewsApi.completed(getApiClient(), {
				days: 7,
				limit: 500,
				contextId: userStateStore.activeContextId ?? undefined
			});
			items = res.items;
			loaded = true;
		} catch (err) {
			toast.error(describeError(err, $t('view.failedLoadCompleted')));
		} finally {
			loading = false;
		}
	}

	async function toggle(): Promise<void> {
		expanded = !expanded;
		if (expanded) await ensureLoaded();
	}

	const mutator = {
		replace(t: Task) {
			items = items.map((x) => (x.id === t.id ? t : x));
		},
		remove(id: number) {
			items = items.filter((x) => x.id !== id);
		}
	};

	async function onItemToggle(task: Task): Promise<void> {
		await toggleComplete(task, mutator, { belongs: () => false });
	}
</script>

<div class="flex flex-col items-stretch gap-2 pt-6">
	<button
		type="button"
		class="mx-auto inline-flex items-center gap-2 rounded-md px-3 py-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground/70 transition-colors hover:bg-accent hover:text-foreground"
		onclick={toggle}
		aria-expanded={expanded}
	>
		<span>{$t('view.completedThisWeek')}</span>
		{#if loaded}
			<span
				class="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-muted px-1.5 text-[11px] font-medium text-muted-foreground"
			>
				{items.length}
			</span>
		{/if}
		<CaretDownIcon class="size-3.5 transition-transform {expanded ? 'rotate-180' : ''}" />
	</button>

	{#if expanded}
		<div class="px-1">
			{#if loading}
				<div class="px-4 py-4 text-sm text-muted-foreground">{$t('app.loading')}</div>
			{:else if items.length === 0}
				<div class="px-4 py-4 text-sm text-muted-foreground">
					{$t('view.noTasksCompletedWeek')}
				</div>
			{:else}
				<div class="flex flex-col gap-4">
					{#each groups as group (group.dayKey)}
						<section>
							<GroupHeader label={group.label} />
							<TaskTree tasks={group.tasks} hideDue onToggle={onItemToggle} />
						</section>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
