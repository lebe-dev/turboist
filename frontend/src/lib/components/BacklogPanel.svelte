<script lang="ts">
	import type { Task } from '$lib/api/types';
	import { planningStore } from '$lib/stores/planning.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import TaskItem from './TaskItem.svelte';
	import SearchIcon from '@lucide/svelte/icons/search';
	import XIcon from '@lucide/svelte/icons/x';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import ChevronsRightIcon from '@lucide/svelte/icons/chevrons-right';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import { t } from 'svelte-intl-precompile';

	let searchQuery = $state('');
	let accepting = $state(false);

	async function handleAcceptAll() {
		if (accepting) return;
		accepting = true;
		try {
			await planningStore.acceptAll();
		} finally {
			accepting = false;
		}
	}

	const backlogLabel = $derived(planningStore.config?.backlog_label ?? '');

	const filteredTasks = $derived(
		searchQuery
			? planningStore.backlogTasks.filter((t) =>
					t.content.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: planningStore.backlogTasks
	);

	const activeContextName = $derived.by(() => {
		const id = contextsStore.activeContextId;
		if (!id) return '';
		return contextsStore.contexts.find((c) => c.id === id)?.display_name ?? '';
	});

	function hasBacklogLabel(task: Task): boolean {
		return backlogLabel !== '' && task.labels.includes(backlogLabel);
	}

	// Reset search when context changes
	$effect(() => {
		contextsStore.activeContextId;
		searchQuery = '';
	});
</script>

<div class="flex h-full flex-col">
	<div class="shrink-0 border-b border-border/50 px-4 py-3">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-2">
				<h2 class="text-xs font-semibold uppercase tracking-wider text-muted-foreground/60">
					{$t('planning.backlog')}
				</h2>
				{#if planningStore.meta.backlog_count > 0}
					<span class="rounded-full bg-muted px-1.5 py-0.5 text-[11px] tabular-nums text-muted-foreground">
						{planningStore.meta.backlog_count}
					</span>
				{/if}
			</div>
			{#if filteredTasks.length > 0}
				<button
					class="flex items-center gap-1 rounded-md px-2 py-1 text-[11px] font-medium text-muted-foreground/60 transition-colors hover:bg-accent hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
					onclick={handleAcceptAll}
					disabled={accepting || planningStore.isAtLimit}
					title={$t('planning.acceptAll')}
				>
					<ChevronsRightIcon class="h-3.5 w-3.5" />
					{$t('planning.acceptAll')}
				</button>
			{/if}
		</div>
		{#if activeContextName}
			<p class="mt-1 text-[11px] text-muted-foreground/50">
				{$t('planning.contextFilter', { values: { name: activeContextName } })}
			</p>
		{/if}
		<div class="relative mt-2 flex items-center">
			<SearchIcon class="pointer-events-none absolute left-2.5 h-3.5 w-3.5 text-muted-foreground/60" />
			<input
				type="text"
				placeholder={$t('tasks.search')}
				bind:value={searchQuery}
				class="h-8 w-full rounded-md border border-border/50 bg-transparent pl-8 pr-8 text-[13px] text-foreground placeholder:text-muted-foreground/50 focus:border-border focus:outline-none"
			/>
			{#if searchQuery}
				<button
					class="absolute right-2 flex items-center text-muted-foreground/60 hover:text-foreground"
					onclick={() => (searchQuery = '')}
					aria-label="Clear search"
				>
					<XIcon class="h-3.5 w-3.5" />
				</button>
			{/if}
		</div>
	</div>

	<div class="flex-1 overflow-y-auto px-1 py-2">
		{#if filteredTasks.length === 0}
			<div class="flex flex-col items-center justify-center py-20 text-muted-foreground">
				<InboxIcon class="mb-3 h-10 w-10 animate-float opacity-20" />
				<p class="text-sm">{$t('tasks.noTasks')}</p>
			</div>
		{:else}
			<div class="space-y-px px-1">
				{#each filteredTasks as task (task.id)}
					<div class="flex items-center">
						<div class="min-w-0 flex-1">
							<TaskItem {task} {searchQuery} />
						</div>
						<button
							class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground/50 transition-colors hover:bg-accent hover:text-foreground disabled:opacity-30 disabled:pointer-events-none"
							onclick={() => planningStore.moveToWeekly(task)}
							disabled={planningStore.isAtLimit}
							aria-label={$t('planning.moveToWeek')}
							title={planningStore.isAtLimit ? $t('planning.limitReached') : $t('planning.moveToWeek')}
						>
							<ArrowRightIcon class="h-4 w-4" />
						</button>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
