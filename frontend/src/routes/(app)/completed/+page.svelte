<script lang="ts">
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import { t } from '$lib/i18n';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import GroupHeader from '$lib/components/view/GroupHeader.svelte';
	import { groupByCompletedDay } from '$lib/utils/viewGroup';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { nowStore } from '$lib/stores/now.svelte';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	const DAYS = 30;

	const list = useListMutator<Task>();
	const { mutator } = list;

	const groups = $derived(
		groupByCompletedDay(list.items, configStore.value?.timezone ?? null, nowStore.now)
	);

	const loader = usePageLoad(
		async (isValid) => {
			const res = await viewsApi.completed(getApiClient(), {
				days: DAYS,
				limit: 500,
				contextId: userStateStore.activeContextId ?? undefined
			});
			if (!isValid()) return;
			list.items = res.items;
		},
		{ errorMessage: $t('page.completed.errorLoading'), autoLoad: false, initialLoading: true }
	);

	$effect(() => {
		void userStateStore.activeContextId;
		void loader.refetch();
	});
</script>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={CheckCircleIcon}
		emptyTitle={$t('page.completed.emptyTitle')}
		emptyDescription={$t('page.completed.emptyDescription')}
	>
		<div class="flex flex-col py-2">
			{#each groups as group, i (group.dayKey)}
				{#if i > 0}
					<hr class="my-4 border-t border-border" />
				{/if}
				<section>
					<GroupHeader label={group.label} />
					<TaskTree
						tasks={group.tasks}
						hideDue
						{mutator}
						belongs={(t) => t.status === 'completed'}
						onToggle={(t) => toggleComplete(t, mutator)}
					/>
				</section>
			{/each}
		</div>
	</ViewContent>
</div>
