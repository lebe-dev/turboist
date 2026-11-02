<script lang="ts">
	import { t } from '$lib/i18n';
	import type { WeekPlanStats } from '$lib/utils/weekPlanStats';

	let { stats, showTroiki }: { stats: WeekPlanStats; showTroiki: boolean } = $props();
</script>

{#if stats.labels.length > 0 || (showTroiki && stats.total > 0)}
	<div
		class="mt-2 flex flex-wrap items-center gap-1.5 text-[11px]"
		data-testid="week-plan-stats"
		aria-label={$t('page.nextWeek.statsLabel')}
	>
		{#if showTroiki}
			<span
				class="inline-flex items-center rounded-full border border-border/60 px-2 py-0.5 font-medium text-foreground"
				data-testid="week-plan-stats-troiki"
			>
				{$t('page.nextWeek.fromTroiki', {
					values: { count: stats.fromTroiki, total: stats.total }
				})}
			</span>
		{/if}
		{#each stats.labels as usage (usage.label.id)}
			<span
				class="inline-flex items-center gap-1 rounded-full bg-accent/50 px-2 py-0.5 font-medium text-muted-foreground"
			>
				{usage.label.name}
				<span class="font-mono text-foreground tabular-nums">{usage.count}</span>
			</span>
		{/each}
	</div>
{/if}
