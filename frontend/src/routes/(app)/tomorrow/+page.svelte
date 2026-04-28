<script lang="ts">
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import type { Task } from '$lib/api/types';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import DayPartSection from '$lib/components/view/DayPartSection.svelte';
	import { groupByDayPart } from '$lib/utils/viewGroup';
	import { parseIso, dayKeyInTz, shiftDayKey } from '$lib/utils/format';
	import { configStore } from '$lib/stores/config.svelte';
	import { toggleComplete } from '$lib/utils/taskActions';
	import { useListMutator } from '$lib/hooks/useListMutator.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';

	let total = $state(0);

	const list = useListMutator<Task>({ onRemove: () => { total = Math.max(0, total - 1); } });
	const { mutator } = list;

	const dayParts = $derived(configStore.value?.dayParts);
	const groups = $derived(groupByDayPart(list.items, dayParts));

	const loader = usePageLoad(async () => {
		const res = await viewsApi.tomorrow(getApiClient());
		list.items = res.items;
		total = res.total;
	}, { errorMessage: 'Failed to load tomorrow' });

	function isTomorrow(t: Task): boolean {
		const dt = parseIso(t.dueAt);
		if (!dt) return false;
		const tz = configStore.value?.timezone ?? null;
		const tomorrowKey = shiftDayKey(dayKeyInTz(new Date(), tz), 1);
		return dayKeyInTz(dt, tz) === tomorrowKey;
	}

</script>

<ViewHeader
	title="Tomorrow"
	subtitle={loader.loading ? 'Loading…' : `${total} task${total === 1 ? '' : 's'}`}
/>

<div class="px-2 py-2">
	<ViewContent
		loading={loader.loading}
		isEmpty={list.items.length === 0}
		emptyIcon={SunHorizonIcon}
		emptyTitle="Nothing for tomorrow"
		emptyDescription="Schedule tasks ahead to see them here."
	>
		<div class="flex flex-col gap-4 py-2">
			{#each groups as group (group.part)}
				<DayPartSection
					part={group.part}
					label={group.label}
					interval={group.interval}
					count={group.tasks.length}
				>
					<TaskTree
						tasks={group.tasks}
						hideDayPart
						{mutator}
						belongs={isTomorrow}
						onToggle={(t) => toggleComplete(t, mutator, { belongs: isTomorrow })}
					/>
				</DayPartSection>
			{/each}
		</div>
	</ViewContent>
</div>
