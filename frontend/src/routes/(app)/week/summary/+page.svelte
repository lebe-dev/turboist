<script lang="ts">
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import ListChecksIcon from 'phosphor-svelte/lib/ListChecks';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import { Button } from '$lib/components/ui/button';
	import { t, locale } from '$lib/i18n';
	import { views as viewsApi } from '$lib/api/endpoints/views';
	import { getApiClient } from '$lib/api/client';
	import { configStore } from '$lib/stores/config.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import type { Task, WeekSummaryResponse } from '$lib/api/types';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import { PRIORITY_ORDER, PRIORITY_LABEL, PRIORITY_COLOR } from '$lib/utils/priority';
	import { formatDayKeyRange, weekRangeKeys } from '$lib/utils/format';
	import { nowStore } from '$lib/stores/now.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';

	let summary = $state<WeekSummaryResponse | null>(null);

	const tz = $derived(configStore.value?.timezone ?? null);
	const weekRange = $derived(weekRangeKeys(nowStore.now, tz));
	const weekRangeLabel = $derived(
		formatDayKeyRange(weekRange.startKey, weekRange.endKey, $locale, tz)
	);

	const completed = $derived(summary?.completed ?? []);

	// Breakdown by priority — fixed P1..P4 order, only buckets with hits shown.
	const byPriority = $derived(
		PRIORITY_ORDER.map((p) => ({
			priority: p,
			count: completed.filter((task) => task.priority === p).length
		})).filter((row) => row.count > 0)
	);

	function projectTitle(projectId: number | null): string {
		if (projectId === null) return $t('page.weekSummary.noProject');
		return projectsStore.items.find((p) => p.id === projectId)?.title
			?? $t('page.weekSummary.noProject');
	}

	function contextName(contextId: number | null): string {
		if (contextId === null) return $t('page.weekSummary.noContext');
		return contextsStore.items.find((c) => c.id === contextId)?.name
			?? $t('page.weekSummary.noContext');
	}

	// Generic count-by helper, sorted by descending count then label.
	function countBy(tasks: Task[], label: (t: Task) => string): { label: string; count: number }[] {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const byLabel = new Map<string, number>();
		for (const task of tasks) {
			const key = label(task);
			byLabel.set(key, (byLabel.get(key) ?? 0) + 1);
		}
		return [...byLabel.entries()]
			.map(([name, count]) => ({ label: name, count }))
			.sort((a, b) => b.count - a.count || a.label.localeCompare(b.label));
	}

	const byProject = $derived(countBy(completed, (task) => projectTitle(task.projectId)));
	const byContext = $derived(countBy(completed, (task) => contextName(task.contextId)));

	const loader = usePageLoad(
		async (isValid) => {
			const res = await viewsApi.weekSummary(getApiClient());
			if (!isValid()) return;
			summary = res;
		},
		{ errorMessage: $t('page.weekSummary.errorLoading') }
	);

	useInvalidation(['tasks'], () => void loader.revalidate());
</script>

<ViewHeader stackOnMobile>
	{#snippet meta()}
		<p class="text-xl font-semibold leading-tight tracking-tight text-foreground sm:text-2xl">
			{$t('page.weekSummary.title')}
		</p>
		<p class="mt-1 text-sm text-muted-foreground">{weekRangeLabel}</p>
	{/snippet}
	{#snippet actions()}
		<Button href="/week" variant="outline" size="sm">
			<ArrowLeftIcon />
			<span>{$t('page.weekSummary.backToWeek')}</span>
		</Button>
	{/snippet}
</ViewHeader>

<div class="px-4 py-2 sm:px-8">
	<ViewContent
		loading={loader.loading}
		isEmpty={summary !== null && summary.stats.completedCount === 0 && summary.stats.plannedOpen === 0 && summary.stats.overdue === 0}
		emptyIcon={ListChecksIcon}
		emptyTitle={$t('page.weekSummary.emptyTitle')}
		emptyDescription={$t('page.weekSummary.emptyDescription')}
	>
		{#if summary}
			<div class="flex flex-col gap-6 py-2">
				<!-- Headline counters -->
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
					<div class="flex items-center gap-3 rounded-lg border bg-card p-4">
						<CheckCircleIcon class="size-8 shrink-0 text-emerald-500" weight="fill" />
						<div>
							<p class="text-2xl font-semibold tabular-nums text-foreground">
								{summary.stats.completedCount}
							</p>
							<p class="text-sm text-muted-foreground">{$t('page.weekSummary.completed')}</p>
						</div>
					</div>
					<div class="flex items-center gap-3 rounded-lg border bg-card p-4">
						<ListChecksIcon class="size-8 shrink-0 text-sky-500" weight="fill" />
						<div>
							<p class="text-2xl font-semibold tabular-nums text-foreground">
								{summary.stats.plannedOpen}
							</p>
							<p class="text-sm text-muted-foreground">{$t('page.weekSummary.plannedOpen')}</p>
						</div>
					</div>
					<div class="flex items-center gap-3 rounded-lg border bg-card p-4">
						<WarningIcon class="size-8 shrink-0 text-amber-500" weight="fill" />
						<div>
							<p class="text-2xl font-semibold tabular-nums text-foreground">
								{summary.stats.overdue}
							</p>
							<p class="text-sm text-muted-foreground">{$t('page.weekSummary.overdue')}</p>
						</div>
					</div>
				</div>

				{#if completed.length > 0}
					<!-- By priority -->
					<section class="rounded-lg border bg-card p-4">
						<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
							{$t('page.weekSummary.byPriority')}
						</h2>
						<ul class="flex flex-col gap-2">
							{#each byPriority as row (row.priority)}
								<li class="flex items-center justify-between text-sm">
									<span class="flex items-center gap-2">
										<span class="text-base leading-none {PRIORITY_COLOR[row.priority]}">●</span>
										<span class="text-foreground">{PRIORITY_LABEL[row.priority]}</span>
									</span>
									<span class="font-medium tabular-nums text-muted-foreground">{row.count}</span>
								</li>
							{/each}
						</ul>
					</section>

					<!-- By project -->
					<section class="rounded-lg border bg-card p-4">
						<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
							{$t('page.weekSummary.byProject')}
						</h2>
						<ul class="flex flex-col gap-2">
							{#each byProject as row (row.label)}
								<li class="flex items-center justify-between gap-3 text-sm">
									<span class="min-w-0 truncate text-foreground">{row.label}</span>
									<span class="font-medium tabular-nums text-muted-foreground">{row.count}</span>
								</li>
							{/each}
						</ul>
					</section>

					<!-- By context -->
					<section class="rounded-lg border bg-card p-4">
						<h2 class="mb-3 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
							{$t('page.weekSummary.byContext')}
						</h2>
						<ul class="flex flex-col gap-2">
							{#each byContext as row (row.label)}
								<li class="flex items-center justify-between gap-3 text-sm">
									<span class="min-w-0 truncate text-foreground">{row.label}</span>
									<span class="font-medium tabular-nums text-muted-foreground">{row.count}</span>
								</li>
							{/each}
						</ul>
					</section>
				{/if}
			</div>
		{/if}
	</ViewContent>
</div>
