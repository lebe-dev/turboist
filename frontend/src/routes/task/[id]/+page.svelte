<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { onMount, onDestroy } from 'svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import TaskDetailPanel from '$lib/components/TaskDetailPanel.svelte';
	import NextActionDialog from '$lib/components/NextActionDialog.svelte';

	const taskId = $derived($page.params.id as string);

	onMount(() => {
		tasksStore.start();
	});

	onDestroy(() => {
		tasksStore.stop();
	});
</script>

<TaskDetailPanel
	{taskId}
	fullPage={true}
	onclose={() => history.back()}
	onselect={(id) => goto(`/task/${id}`)}
/>
<NextActionDialog />
