<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import TaskDetailPanel from '$lib/components/TaskDetailPanel.svelte';
	import NextActionDialog from '$lib/components/NextActionDialog.svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';

	const taskId = $derived($page.params.id as string);

	// Redirect reconciled temp IDs to real IDs
	$effect(() => {
		if (taskId.startsWith('temp-')) {
			const realId = tasksStore.resolveTaskId(taskId);
			if (realId) goto(`/task/${realId}`, { replaceState: true });
		}
	});
</script>

{#key taskId}
<TaskDetailPanel
	{taskId}
	fullPage={true}
	onclose={() => history.back()}
	onselect={(id) => goto(`/task/${id}`)}
/>
{/key}
<NextActionDialog />
