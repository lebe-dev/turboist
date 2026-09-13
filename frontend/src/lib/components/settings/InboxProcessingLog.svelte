<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import ArrowUUpLeftIcon from 'phosphor-svelte/lib/ArrowUUpLeft';
	import { resolve } from '$app/paths';
	import { t, locale } from '$lib/i18n';
	import { getApiClient } from '$lib/api/client';
	import { inboxProcessing as inboxProcessingApi } from '$lib/api/endpoints/inbox-processing';
	import type { InboxProcessingLogEntry, InboxProcessingOutcome } from '$lib/api/types';
	import { Button } from '$lib/components/ui/button';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import { describeError } from '$lib/utils/taskActions';

	const LOG_LIMIT = 50;

	// Bumped by the parent after a manual run so the journal shows its result.
	let { refreshKey = 0 }: { refreshKey?: number } = $props();

	let entries = $state<InboxProcessingLogEntry[]>([]);
	let loaded = $state(false);
	let loadError = $state(false);
	let revertingId = $state<number | null>(null);

	async function load(): Promise<void> {
		try {
			const page = await inboxProcessingApi.log(getApiClient(), LOG_LIMIT, 0);
			entries = page.items;
			loadError = false;
		} catch {
			loadError = true;
		} finally {
			loaded = true;
		}
	}

	onMount(() => {
		void load();
	});

	// A background run that filed something publishes `tasks`.
	useInvalidation(['tasks'], () => void load());

	let lastRefreshKey = untrack(() => refreshKey);
	$effect(() => {
		const key = refreshKey;
		if (key === lastRefreshKey) return;
		lastRefreshKey = key;
		untrack(() => void load());
	});

	async function revert(entry: InboxProcessingLogEntry): Promise<void> {
		if (revertingId !== null) return;
		revertingId = entry.id;
		try {
			await inboxProcessingApi.revert(getApiClient(), entry.id);
			toast.success($t('settings.inboxProcessing.toasts.reverted'));
			await load();
		} catch (err) {
			toast.error(describeError(err, $t('settings.inboxProcessing.toasts.revertFailed')));
		} finally {
			revertingId = null;
		}
	}

	function projectTitle(id: number): string {
		return projectsStore.items.find((p) => p.id === id)?.title ?? $t('settings.inboxProcessing.log.deleted');
	}

	function labelName(id: number): string {
		return labelsStore.items.find((l) => l.id === id)?.name ?? $t('settings.inboxProcessing.log.deleted');
	}

	// Labels the decision added — those the task already carried are not news.
	function addedLabels(entry: InboxProcessingLogEntry): number[] {
		if (!entry.after) return [];
		const before = new Set(entry.before.labelIds);
		return entry.after.labelIds.filter((id) => !before.has(id));
	}

	function formatDateTime(iso: string): string {
		try {
			return new Intl.DateTimeFormat($locale ?? undefined, {
				dateStyle: 'short',
				timeStyle: 'short',
				timeZone: configStore.value?.timezone ?? undefined
			}).format(new Date(iso));
		} catch {
			return iso;
		}
	}

	function formatDate(iso: string): string {
		try {
			return new Intl.DateTimeFormat($locale ?? undefined, {
				dateStyle: 'medium',
				timeZone: configStore.value?.timezone ?? undefined
			}).format(new Date(iso));
		} catch {
			return iso;
		}
	}

	function outcomeClass(outcome: InboxProcessingOutcome): string {
		switch (outcome) {
			case 'sorted':
				return 'bg-green-500/15 text-green-700 dark:text-green-300';
			case 'kept':
				return 'bg-muted text-muted-foreground';
			case 'failed':
				return 'bg-red-500/15 text-red-700 dark:text-red-300';
		}
	}
</script>

<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex flex-col gap-0.5">
		<h2 class="text-sm font-semibold">{$t('settings.inboxProcessing.log.heading')}</h2>
		<p class="text-xs text-muted-foreground">{$t('settings.inboxProcessing.log.description')}</p>
	</div>

	{#if loadError && entries.length === 0}
		<p class="py-4 text-center text-sm text-destructive">{$t('settings.inboxProcessing.log.loadFailed')}</p>
	{:else if loaded && entries.length === 0}
		<p class="py-4 text-center text-sm text-muted-foreground">{$t('settings.inboxProcessing.log.empty')}</p>
	{:else}
		<ul class="flex flex-col divide-y divide-border overflow-hidden rounded-md border border-border">
			{#each entries as entry (entry.id)}
				<li class="flex flex-col gap-1.5 px-3 py-2.5 text-sm" data-testid="inbox-log-entry">
					<div class="flex flex-wrap items-center gap-2">
						<span class="rounded px-1.5 py-0.5 text-[11px] font-medium {outcomeClass(entry.outcome)}">
							{$t(`settings.inboxProcessing.log.outcome.${entry.outcome}`)}
						</span>
						{#if entry.revertedAt}
							<span class="rounded bg-yellow-500/15 px-1.5 py-0.5 text-[11px] font-medium text-yellow-700 dark:text-yellow-300">
								{$t('settings.inboxProcessing.log.reverted')}
							</span>
						{/if}
						{#if entry.taskId !== null}
							<a
								class="min-w-0 flex-1 truncate font-medium hover:underline"
								href={resolve('/(app)/task/[id]', { id: String(entry.taskId) })}
							>
								{entry.taskTitle}
							</a>
						{:else}
							<span class="min-w-0 flex-1 truncate font-medium text-muted-foreground line-through" title={$t('settings.inboxProcessing.log.deletedTask')}>
								{entry.taskTitle}
							</span>
						{/if}
						<time class="shrink-0 text-xs text-muted-foreground" datetime={entry.createdAt}>
							{formatDateTime(entry.createdAt)}
						</time>
					</div>

					{#if entry.outcome === 'sorted' && entry.after}
						<div class="flex flex-wrap items-center gap-1.5 text-xs">
							<span class="rounded-full border border-border px-2 py-0.5">{projectTitle(entry.after.projectId)}</span>
							{#each addedLabels(entry) as labelId (labelId)}
								<span class="rounded-full bg-muted px-2 py-0.5 text-muted-foreground">#{labelName(labelId)}</span>
							{/each}
							{#if entry.after.dueAt && entry.after.dueAt !== entry.before.dueAt}
								<span class="text-muted-foreground">
									{$t('settings.inboxProcessing.log.due', { values: { date: formatDate(entry.after.dueAt) } })}
								</span>
							{/if}
						</div>
					{/if}

					{#if entry.reason}
						<p class="text-xs text-muted-foreground">{entry.reason}</p>
					{/if}
					{#if entry.error}
						<p class="break-words font-mono text-xs text-red-600 dark:text-red-400">{entry.error}</p>
					{/if}

					<div class="flex flex-wrap items-center justify-between gap-2 text-[11px] text-muted-foreground">
						<span>
							{entry.model}{#if entry.confidence !== null}
								· {$t('settings.inboxProcessing.log.confidence')} {Math.round(entry.confidence * 100)}%{/if}
						</span>
						{#if entry.outcome === 'sorted' && !entry.revertedAt && entry.taskId !== null}
							<Button
								variant="outline"
								size="sm"
								disabled={revertingId !== null}
								onclick={() => revert(entry)}
							>
								<ArrowUUpLeftIcon class="size-3.5" />
								{$t('settings.inboxProcessing.log.revert')}
							</Button>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</section>
