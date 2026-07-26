<script lang="ts">
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import XIcon from 'phosphor-svelte/lib/X';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import ArrowRightIcon from 'phosphor-svelte/lib/ArrowRight';
	import LinkIcon from 'phosphor-svelte/lib/Link';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import { getApiClient } from '$lib/api/client';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import type { RelationDirection, RelationType, Task, TaskRelation } from '$lib/api/types';
	import { describeError } from '$lib/utils/taskActions';
	import AddTaskRelationDialog from '$lib/components/dialog/AddTaskRelationDialog.svelte';
	import { t } from '$lib/i18n';

	let {
		task,
		onUpdated
	}: {
		task: Task;
		/** Receives the updated task the mutation answers with — no follow-up read. */
		onUpdated: (updated: Task) => void;
	} = $props();

	let addOpen = $state(false);
	let busyRelationId = $state<number | null>(null);

	const relations = $derived(task.relations ?? []);
	const peerIds = $derived(relations.map((r) => r.task.id));

	// Three visual groups over one stored edge type: an `incoming` blocks edge is
	// "blocked by", an `outgoing` one is "blocks", and `related` is direction-free.
	const groups = $derived([
		{
			key: 'blockedBy',
			items: relations.filter((r) => r.type === 'blocks' && r.direction === 'incoming')
		},
		{
			key: 'blocks',
			items: relations.filter((r) => r.type === 'blocks' && r.direction === 'outgoing')
		},
		{ key: 'related', items: relations.filter((r) => r.type === 'related') }
	]);

	async function add(
		targetTaskId: number,
		type: RelationType,
		direction: RelationDirection
	): Promise<void> {
		try {
			onUpdated(await tasksApi.addRelation(getApiClient(), task.id, { targetTaskId, type, direction }));
		} catch (err) {
			toast.error(describeError(err, $t('page.task.relationAddFailed')));
		}
	}

	async function remove(relation: TaskRelation): Promise<void> {
		busyRelationId = relation.id;
		try {
			onUpdated(await tasksApi.removeRelation(getApiClient(), task.id, relation.id));
		} catch (err) {
			toast.error(describeError(err, $t('page.task.relationRemoveFailed')));
		} finally {
			busyRelationId = null;
		}
	}

</script>

<section class="flex flex-col gap-2">
	<div class="flex items-center justify-between gap-2">
		<span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
			{$t('page.task.relations')}
		</span>
		<button
			type="button"
			onclick={() => (addOpen = true)}
			class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
		>
			<PlusIcon class="size-3" />
			{$t('page.task.addRelation')}
		</button>
	</div>

	{#if relations.length === 0}
		<p class="text-xs text-muted-foreground">{$t('page.task.relationEmpty')}</p>
	{:else}
		{#each groups as group (group.key)}
			{#if group.items.length > 0}
				<div class="flex flex-col gap-0.5">
					<span class="px-1 text-[10px] text-muted-foreground">
						{$t(`page.task.relation_${group.key}`)}
					</span>
					{#each group.items as relation (relation.id)}
						{@const peer = relation.task}
						{@const done = peer.status === 'completed' || peer.status === 'cancelled'}
						<div class="group/rel flex items-center gap-2 rounded px-1 py-1 hover:bg-accent/50">
							{#if group.key === 'blockedBy'}
								<LockSimpleIcon
									class="size-3 shrink-0 {done ? 'text-muted-foreground/40' : 'text-amber-500'}"
								/>
							{:else if group.key === 'blocks'}
								<ArrowRightIcon class="size-3 shrink-0 text-muted-foreground/60" />
							{:else}
								<LinkIcon class="size-3 shrink-0 text-muted-foreground/60" />
							{/if}
							<a
								href={resolve('/(app)/task/[id]', { id: String(peer.id) })}
								class="min-w-0 flex-1 truncate text-sm"
								class:line-through={done}
								class:text-muted-foreground={done}
							>
								{peer.title}
							</a>
							{#if done}
								<CheckIcon class="size-3 shrink-0 text-muted-foreground" weight="bold" />
							{/if}
							<span class="shrink-0 text-[10px] text-muted-foreground/70">#{peer.id}</span>
							<button
								type="button"
								onclick={() => void remove(relation)}
								disabled={busyRelationId === relation.id}
								aria-label={$t('page.task.relationRemove')}
								title={$t('page.task.relationRemove')}
								class="shrink-0 rounded p-0.5 text-muted-foreground opacity-0 transition-opacity hover:bg-accent hover:text-foreground focus-visible:opacity-100 group-hover/rel:opacity-100 disabled:opacity-50"
							>
								<XIcon class="size-3" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		{/each}
	{/if}
</section>

<AddTaskRelationDialog
	bind:open={addOpen}
	currentTaskId={task.id}
	excludeTaskIds={peerIds}
	onSelect={add}
/>
