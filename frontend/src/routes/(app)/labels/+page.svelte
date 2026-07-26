<script lang="ts">
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import ChartBarIcon from 'phosphor-svelte/lib/ChartBar';
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import BroomIcon from 'phosphor-svelte/lib/Broom';
	import TrendUpIcon from 'phosphor-svelte/lib/TrendUp';
	import TrendDownIcon from 'phosphor-svelte/lib/TrendDown';
	import { resolve } from '$app/paths';
	import { getApiClient } from '$lib/api/client';
	import { labels as labelsApi } from '$lib/api/endpoints/labels';
	import type { LabelStatsPeriod, LabelStatsResponse } from '$lib/api/types';
	import ViewHeader from '$lib/components/view/ViewHeader.svelte';
	import ViewContent from '$lib/components/view/ViewContent.svelte';
	import { usePageLoad } from '$lib/hooks/usePageLoad.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { nowStore } from '$lib/stores/now.svelte';
	import { isLabelVisible } from '$lib/utils/visibility';
	import {
		buildLabelStatsRows,
		labelStatsTotals,
		lastUsedDaysAgo,
		splitLabelStatsRows
	} from '$lib/utils/labelStats';
	import { dayKeyInTz, formatDayKeyRange, parseIso } from '$lib/utils/format';
	import { t, locale } from '$lib/i18n';

	const PERIODS: LabelStatsPeriod[] = ['week', 'month', 'quarter'];
	const PERIOD_LABEL: Record<LabelStatsPeriod, string> = {
		week: 'page.labels.period.week',
		month: 'page.labels.period.month',
		quarter: 'page.labels.period.quarter'
	};
	const PERIOD_RANGE_LABEL: Record<LabelStatsPeriod, string> = {
		week: 'page.labels.rangeDays.week',
		month: 'page.labels.rangeDays.month',
		quarter: 'page.labels.rangeDays.quarter'
	};

	let stats = $state<LabelStatsResponse | null>(null);
	let period = $state<LabelStatsPeriod>('week');

	const tz = $derived(configStore.value?.timezone ?? null);

	// A stale cached payload is served offline, so the window is read from the
	// response rather than recomputed locally — the header then describes the data
	// actually on screen.
	const range = $derived(stats?.ranges[period] ?? null);
	const rangeLabel = $derived.by(() => {
		if (!range) return '';
		const start = parseIso(range.start);
		const end = parseIso(range.end);
		if (!start || !end) return '';
		// Both the range and formatDayKeyRange treat the end as exclusive.
		return formatDayKeyRange(dayKeyInTz(start, tz), dayKeyInTz(end, tz), $locale, tz);
	});

	const rows = $derived(
		buildLabelStatsRows(
			(stats?.items ?? []).filter((i) => isLabelVisible(i.label, settingsStore.publicView)),
			period
		)
	);
	const split = $derived(splitLabelStatsRows(rows));
	const activeRows = $derived(split.active);
	const idleRows = $derived(split.idle);
	const maxApplied = $derived(Math.max(1, ...activeRows.map((r) => r.applied)));

	const totals = $derived(labelStatsTotals(rows));

	function barWidth(applied: number): number {
		return Math.max(2, Math.round((applied / maxApplied) * 100));
	}

	// Age of the most recent application: today / yesterday / N days ago / never.
	function lastUsedLabel(lastUsedAt: string | null): string {
		const days = lastUsedDaysAgo(lastUsedAt, tz, nowStore.now);
		if (days === null) return $t('page.labels.lastUsed.never');
		if (days === 0) return $t('page.labels.lastUsed.today');
		if (days === 1) return $t('page.labels.lastUsed.yesterday');
		return $t('page.labels.lastUsed.daysAgo', { values: { days } });
	}

	const loader = usePageLoad(
		async (isValid) => {
			const res = await labelsApi.stats(getApiClient());
			if (!isValid()) return;
			stats = res;
		},
		{ errorMessage: $t('page.labels.errorLoading') }
	);

	// Tagging a task and completing one both move these numbers. Reconnect and
	// outbox-drain catch-ups arrive through the same channel: the layout replays
	// every scope after them.
	useInvalidation(['tasks', 'labels'], () => void loader.revalidate());
</script>

<ViewHeader stackOnMobile>
	{#snippet meta()}
		<p class="text-xl font-semibold leading-tight tracking-tight text-foreground sm:text-2xl">
			{$t('page.labels.title')}
		</p>
		<p class="mt-1 text-sm text-muted-foreground">
			{$t(PERIOD_RANGE_LABEL[period])}{rangeLabel ? ` · ${rangeLabel}` : ''}
		</p>
	{/snippet}
	{#snippet actions()}
		<div
			class="flex items-center gap-1 rounded-md border border-border bg-card p-0.5"
			role="radiogroup"
			aria-label={$t('page.labels.periodAria')}
		>
			{#each PERIODS as value (value)}
				{@const active = period === value}
				<button
					type="button"
					role="radio"
					aria-checked={active}
					onclick={() => (period = value)}
					class="rounded px-2.5 py-1 text-[13px] transition-colors {active
						? 'bg-muted font-medium text-foreground'
						: 'text-muted-foreground hover:text-foreground'}"
				>
					{$t(PERIOD_LABEL[value])}
				</button>
			{/each}
		</div>
	{/snippet}
</ViewHeader>

<div class="px-4 py-2 sm:px-8">
	<ViewContent
		loading={loader.loading}
		isEmpty={stats !== null && rows.length === 0}
		emptyIcon={TagIcon}
		emptyTitle={$t('page.labels.emptyTitle')}
		emptyDescription={$t('page.labels.emptyDescription')}
	>
		<div class="flex flex-col gap-6 py-2">
			<!-- Headline counters for the selected window -->
			<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
				<div class="flex items-center gap-3 rounded-lg border bg-card p-4">
					<ChartBarIcon class="size-8 shrink-0 text-sky-500" weight="fill" />
					<div class="min-w-0">
						<p class="text-2xl font-semibold tabular-nums text-foreground">{totals.applied}</p>
						<p class="text-sm text-muted-foreground">{$t('page.labels.stats.applied')}</p>
					</div>
				</div>
				<div class="flex items-center gap-3 rounded-lg border bg-card p-4">
					<TagIcon class="size-8 shrink-0 text-violet-500" weight="fill" />
					<div class="min-w-0">
						<p class="text-2xl font-semibold tabular-nums text-foreground">
							{activeRows.length}<span class="text-base font-normal text-muted-foreground"
								>/{rows.length}</span
							>
						</p>
						<p class="text-sm text-muted-foreground">{$t('page.labels.stats.active')}</p>
					</div>
				</div>
				<div class="flex items-center gap-3 rounded-lg border bg-card p-4">
					<CheckCircleIcon class="size-8 shrink-0 text-emerald-500" weight="fill" />
					<div class="min-w-0">
						<p class="text-2xl font-semibold tabular-nums text-foreground">{totals.completed}</p>
						<p class="text-sm text-muted-foreground">{$t('page.labels.stats.completed')}</p>
					</div>
				</div>
				<div class="flex items-center gap-3 rounded-lg border bg-card p-4">
					<WarningIcon class="size-8 shrink-0 text-amber-500" weight="fill" />
					<div class="min-w-0">
						<p class="text-2xl font-semibold tabular-nums text-foreground">{totals.overdue}</p>
						<p class="text-sm text-muted-foreground">{$t('page.labels.stats.overdue')}</p>
					</div>
				</div>
			</div>

			<!-- Frequency ranking: most-used label first -->
			<section class="rounded-lg border bg-card p-4">
				<div class="mb-1 flex items-baseline justify-between gap-3">
					<h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
						{$t('page.labels.rankingTitle')}
					</h2>
					<span class="text-xs text-muted-foreground">{$t('page.labels.appliedHint')}</span>
				</div>
				{#if activeRows.length === 0}
					<p class="py-3 text-sm text-muted-foreground">{$t('page.labels.noActivity')}</p>
				{:else}
					<ul class="flex flex-col divide-y divide-border/60">
						{#each activeRows as row (row.item.label.id)}
							<li class="flex flex-col gap-1.5 py-3 first:pt-1">
								<div class="flex items-baseline justify-between gap-3">
									<a
										href={resolve('/(app)/label/[id]', { id: String(row.item.label.id) })}
										class="flex min-w-0 items-center gap-2 text-sm text-foreground hover:underline"
									>
										<TagIcon
											class="size-3.5 shrink-0"
											style={`color: ${row.item.label.color}`}
											weight="fill"
										/>
										<span class="truncate font-medium">{row.item.label.name}</span>
									</a>
									<span class="flex shrink-0 items-center gap-2 tabular-nums">
										<span class="text-sm font-semibold text-foreground">{row.applied}</span>
										{#if row.delta > 0}
											<span
												class="flex items-center gap-0.5 text-xs text-emerald-600 dark:text-emerald-400"
												title={$t('page.labels.trendTooltip', {
													values: { previous: row.previousApplied }
												})}
											>
												<TrendUpIcon class="size-3" weight="bold" />{row.delta}
											</span>
										{:else if row.delta < 0}
											<span
												class="flex items-center gap-0.5 text-xs text-muted-foreground"
												title={$t('page.labels.trendTooltip', {
													values: { previous: row.previousApplied }
												})}
											>
												<TrendDownIcon class="size-3" weight="bold" />{Math.abs(row.delta)}
											</span>
										{/if}
									</span>
								</div>
								<div class="h-1.5 w-full overflow-hidden rounded-full bg-muted">
									<div
										class="h-full rounded-full bg-primary/70"
										style="width: {barWidth(row.applied)}%"
									></div>
								</div>
								<div class="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
									<span>{$t('page.labels.row.completed', { values: { count: row.completed } })}</span>
									<span>{$t('page.labels.row.open', { values: { count: row.item.openTasks } })}</span>
									{#if row.item.overdue > 0}
										<span class="text-amber-600 dark:text-amber-400">
											{$t('page.labels.row.overdue', { values: { count: row.item.overdue } })}
										</span>
									{/if}
									<span>{$t('page.labels.row.total', { values: { count: row.item.totalTasks } })}</span>
									{#if row.item.projects > 0}
										<span
											>{$t('page.labels.row.projects', { values: { count: row.item.projects } })}</span
										>
									{/if}
								</div>
							</li>
						{/each}
					</ul>
				{/if}
			</section>

			<!-- Cleanup candidates: no activity in the window at all -->
			{#if idleRows.length > 0}
				<section class="rounded-lg border bg-card p-4">
					<div class="mb-1 flex items-center gap-2">
						<BroomIcon class="size-4 shrink-0 text-muted-foreground" />
						<h2 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
							{$t('page.labels.idleTitle', { values: { count: idleRows.length } })}
						</h2>
					</div>
					<p class="mb-3 text-xs text-muted-foreground">{$t('page.labels.idleDescription')}</p>
					<ul class="flex flex-col divide-y divide-border/60">
						{#each idleRows as row (row.item.label.id)}
							<li class="flex items-center justify-between gap-3 py-2 text-sm">
								<a
									href={resolve('/(app)/label/[id]', { id: String(row.item.label.id) })}
									class="flex min-w-0 items-center gap-2 text-foreground hover:underline"
								>
									<TagIcon
										class="size-3.5 shrink-0"
										style={`color: ${row.item.label.color}`}
										weight="fill"
									/>
									<span class="truncate">{row.item.label.name}</span>
								</a>
								<span class="shrink-0 text-xs text-muted-foreground">
									{lastUsedLabel(row.item.lastUsedAt)}
								</span>
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		</div>
	</ViewContent>
</div>
