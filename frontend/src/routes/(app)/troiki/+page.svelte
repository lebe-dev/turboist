<script lang="ts">
	import { toast } from 'svelte-sonner';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import PlayIcon from 'phosphor-svelte/lib/Play';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import type { Task, TaskInput, TroikiCategory, TroikiSlot } from '$lib/api/types';
	import { troikiStore } from '$lib/stores/troiki.svelte';
	import { Button } from '$lib/components/ui/button';
	import TaskTree from '$lib/components/task/TaskTree.svelte';
	import QuickAddDialog from '$lib/components/task/QuickAddDialog.svelte';
	import { describeError, toggleComplete } from '$lib/utils/taskActions';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import type { ListMutator } from '$lib/utils/taskActions';

	let subtasksByParent = $state<Record<number, Task[]>>({});

	async function loadSubtasksFor(parentIds: number[]): Promise<void> {
		if (parentIds.length === 0) {
			subtasksByParent = {};
			return;
		}
		const client = getApiClient();
		const results = await Promise.all(
			parentIds.map(async (id) => {
				const page = await tasksApi.listSubtasks(client, id);
				return [id, page.items] as const;
			})
		);
		const next: Record<number, Task[]> = {};
		for (const [id, items] of results) next[id] = items;
		subtasksByParent = next;
	}

	function parentIdsFromView(): number[] {
		const v = troikiStore.value;
		return [...v.important.tasks, ...v.medium.tasks, ...v.rest.tasks].map((t) => t.id);
	}

	async function loadAll(): Promise<void> {
		await troikiStore.load();
		await loadSubtasksFor(parentIdsFromView());
	}

	const loader = usePageLoad(loadAll, { errorMessage: 'Failed to load Troiki' });

	const view = $derived(troikiStore.value);

	const sections: Array<{ key: TroikiCategory; label: string; description: string }> = [
		{ key: 'important', label: 'Important', description: 'Top three tasks that demand focus.' },
		{ key: 'medium', label: 'Medium', description: 'Earned by completing Important tasks.' },
		{ key: 'rest', label: 'Rest', description: 'Earned by completing Medium tasks.' }
	];

	function slotFor(key: TroikiCategory): TroikiSlot {
		return view[key];
	}

	// One mutator per parent row: parent-level changes refetch the whole view
	// (capacity / ordering / subtree all need to be re-derived); subtask changes
	// patch the local subtasksByParent map surgically.
	function treeMutator(parent: Task): ListMutator {
		return {
			replace(t: Task) {
				if (t.id === parent.id) {
					void loadAll();
					return;
				}
				const list = subtasksByParent[parent.id] ?? [];
				subtasksByParent = {
					...subtasksByParent,
					[parent.id]: list.map((x) => (x.id === t.id ? t : x))
				};
			},
			remove(id: number) {
				if (id === parent.id) {
					void loadAll();
					return;
				}
				const list = subtasksByParent[parent.id] ?? [];
				subtasksByParent = {
					...subtasksByParent,
					[parent.id]: list.filter((x) => x.id !== id)
				};
			}
		};
	}

	async function onParentToggle(task: Task): Promise<void> {
		const client = getApiClient();
		try {
			if (task.status === 'completed') {
				await tasksApi.uncomplete(client, task.id);
			} else {
				await tasksApi.complete(client, task.id);
			}
			await loadAll();
		} catch (err) {
			toast.error(describeError(err, 'Failed to update task'));
		}
	}

	function tasksFor(parent: Task): Task[] {
		const subs = subtasksByParent[parent.id] ?? [];
		return [parent, ...subs];
	}

	function onTreeToggle(parent: Task, t: Task): void {
		if (t.id === parent.id) {
			void onParentToggle(t);
			return;
		}
		void toggleComplete(t, treeMutator(parent), { removeWhenCompleted: false });
	}

	let starting = $state(false);
	const canStart = $derived(!view.started && view.important.tasks.length > 0);

	let addOpen = $state(false);
	let addCategory = $state<TroikiCategory>('important');

	function openAdd(category: TroikiCategory): void {
		addCategory = category;
		addOpen = true;
	}

	async function onAddSubmit(
		payload: TaskInput,
		target: { projectId: number | null }
	): Promise<void> {
		const client = getApiClient();
		let created: Task;
		try {
			created =
				target.projectId !== null
					? await projectsApi.createTask(client, target.projectId, payload)
					: await tasksApi.createInbox(client, payload);
		} catch (err) {
			toast.error(describeError(err, 'Failed to create task'));
			return;
		}
		try {
			await tasksApi.setTroikiCategory(client, created.id, addCategory);
			toast.success(`Task added to ${addCategory}`);
		} catch (err) {
			toast.error(describeError(err, 'Created, but failed to assign Troiki category'));
		}
		await loadAll();
	}

	async function startSystem(): Promise<void> {
		if (!canStart || starting) return;
		starting = true;
		try {
			await troikiStore.start();
			await loadSubtasksFor(parentIdsFromView());
			toast.success('Troiki started');
		} catch (err) {
			toast.error(describeError(err, 'Failed to start Troiki'));
		} finally {
			starting = false;
		}
	}
</script>

<div class="px-2 py-2">
	{#if loader.loading}
		<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
	{:else}
		<header class="flex items-center justify-between px-3 pb-1">
			<div class="text-xs text-muted-foreground">
				{#if view.started}
					Cycle in progress — Medium and Rest unlock as you complete the previous category.
				{:else}
					Initial fill — pick your three Important tasks, then optionally seed Medium and Rest. Press
					Start to lock in the cycle.
				{/if}
			</div>
			{#if !view.started}
				<Button
					size="sm"
					variant="default"
					disabled={!canStart || starting}
					onclick={startSystem}
					title={canStart ? 'Start the Troiki cycle' : 'Add at least one Important task first'}
				>
					<PlayIcon class="size-4" weight="fill" />
					{starting ? 'Starting…' : 'Start the system'}
				</Button>
			{/if}
		</header>
		<div class="flex flex-col gap-6 py-2">
			{#each sections as section (section.key)}
				{@const slot = slotFor(section.key)}
				{@const initialMode =
					!view.started &&
					(section.key === 'medium' || section.key === 'rest')}
				{@const locked = !initialMode && slot.capacity === 0}
				{@const open = slot.tasks.length}
				{@const cap = slot.capacity}
				{@const emptySlots = Math.max(0, cap - open)}
				{@const canAdd = !locked && (initialMode || open < cap)}
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
							{:else if initialMode}
								<span
									class="rounded-full border border-dashed border-border bg-muted/20 px-2 py-0.5 text-[11px] uppercase tracking-wide text-muted-foreground"
									title="Open during initial fill — capacity is locked in when you press Start"
								>
									Open · {open}
								</span>
							{:else}
								<span
									class="rounded-full border border-border bg-muted/30 px-2 py-0.5 text-[11px] tabular-nums text-muted-foreground"
								>
									{open}/{cap}
								</span>
							{/if}
						</div>
						<div class="flex items-center gap-2">
							<p class="hidden text-xs text-muted-foreground sm:block">{section.description}</p>
							{#if canAdd}
								<Button
									size="sm"
									variant="ghost"
									class="h-7 px-2 text-xs"
									onclick={() => openAdd(section.key)}
									aria-label={`Add task to ${section.label}`}
								>
									<PlusIcon class="size-3.5" />
									Add task
								</Button>
							{/if}
						</div>
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
					{:else if initialMode && open === 0}
						<div
							class="mx-3 rounded-md border border-dashed border-border/70 bg-muted/10 px-3 py-4 text-xs text-muted-foreground"
						>
							Use the task actions menu (⋯ → Troiki System) to assign tasks to this section before
							starting the cycle.
						</div>
					{:else}
						<div class="flex flex-col divide-y divide-border/40">
							{#each slot.tasks as task (task.id)}
								<TaskTree
									tasks={tasksFor(task)}
									showProject={false}
									hideDue
									mutator={treeMutator(task)}
									onToggle={(t) => onTreeToggle(task, t)}
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

<QuickAddDialog bind:open={addOpen} onSubmit={onAddSubmit} />
