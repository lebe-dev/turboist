<script lang="ts">
	import { toast } from 'svelte-sonner';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { getApiClient } from '$lib/api/client';
	import type { Task, TroikiCategory, TroikiSlot } from '$lib/api/types';
	import { troikiStore } from '$lib/stores/troiki.svelte';
	import TaskItem from '$lib/components/task/TaskItem.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import type { ListMutator } from '$lib/utils/taskActions';

	const loader = usePageLoad(async () => {
		await troikiStore.load();
	}, { errorMessage: 'Failed to load Troiki' });

	const view = $derived(troikiStore.value);

	const sections: Array<{ key: TroikiCategory; label: string; description: string }> = [
		{ key: 'important', label: 'Important', description: 'Top three tasks that demand focus.' },
		{ key: 'medium', label: 'Medium', description: 'Earned by completing Important tasks.' },
		{ key: 'rest', label: 'Rest', description: 'Earned by completing Medium tasks.' }
	];

	function slotFor(key: TroikiCategory): TroikiSlot {
		return view[key];
	}

	const mutator: ListMutator = {
		replace(_t: Task) {
			void troikiStore.load();
		},
		remove(_id: number) {
			void troikiStore.load();
		}
	};

	async function onToggle(task: Task): Promise<void> {
		const client = getApiClient();
		try {
			if (task.status === 'completed') {
				await tasksApi.uncomplete(client, task.id);
			} else {
				await tasksApi.complete(client, task.id);
			}
			await troikiStore.load();
		} catch (err) {
			toast.error(describeError(err, 'Failed to update task'));
		}
	}
</script>

<div class="px-2 py-2">
	{#if loader.loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else}
		<div class="flex flex-col gap-6 py-2">
			{#each sections as section (section.key)}
				{@const slot = slotFor(section.key)}
				{@const locked = slot.capacity === 0}
				{@const open = slot.tasks.length}
				{@const cap = slot.capacity}
				{@const emptySlots = Math.max(0, cap - open)}
				<section>
					<header class="flex items-baseline justify-between px-3 pb-2">
						<div class="flex items-center gap-2">
							<h2 class="text-sm font-semibold uppercase tracking-wide text-foreground">
								{section.label}
							</h2>
							{#if locked}
								<span
									class="inline-flex items-center gap-1 rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[11px] uppercase tracking-wide text-muted-foreground"
									aria-label="Locked"
									title="Locked — earn capacity by completing the previous category"
								>
									<LockSimpleIcon class="size-3" />
									<span>Locked</span>
								</span>
							{:else}
								<span
									class="rounded-full border border-border bg-muted/30 px-2 py-0.5 text-[11px] tabular-nums text-muted-foreground"
								>
									{open}/{cap}
								</span>
							{/if}
						</div>
						<p class="hidden text-xs text-muted-foreground sm:block">{section.description}</p>
					</header>

					{#if locked}
						<div
							class="mx-3 rounded-md border border-dashed border-border/70 bg-muted/20 px-3 py-4 text-xs text-muted-foreground"
						>
							{#if section.key === 'medium'}
								Complete an Important task to unlock a Medium slot.
							{:else}
								Complete a Medium task to unlock a Rest slot.
							{/if}
						</div>
					{:else}
						<div class="flex flex-col divide-y divide-border/40">
							{#each slot.tasks as task (task.id)}
								<TaskItem
									{task}
									showProject={false}
									hideDue
									{mutator}
									onToggle={(t) => onToggle(t)}
								/>
							{/each}
							{#each Array.from({ length: emptySlots }) as _, i (i)}
								<div
									class="flex items-center gap-3 rounded-lg border border-dashed border-border/40 px-3 py-2.5 text-xs text-muted-foreground/70"
								>
									<span class="inline-block size-4 shrink-0 rounded-full border border-dashed border-border/70"></span>
									<span>Empty slot</span>
								</div>
							{/each}
						</div>
					{/if}
				</section>
			{/each}
		</div>
	{/if}
</div>
