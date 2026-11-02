<script lang="ts">
	import { t } from '$lib/i18n';
	import type { WeekPlanStats } from '$lib/utils/weekPlanStats';

	let { stats, showTroiki }: { stats: WeekPlanStats; showTroiki: boolean } = $props();

	// A label without a colour of its own still needs a distinct segment.
	const FALLBACK_COLORS = [
		'oklch(0.65 0.15 250)',
		'oklch(0.7 0.15 150)',
		'oklch(0.72 0.15 70)',
		'oklch(0.65 0.18 20)',
		'oklch(0.62 0.15 300)',
		'oklch(0.7 0.12 200)'
	];
	const LEGEND_LIMIT = 6;

	interface Segment {
		key: string;
		name: string;
		count: number;
		color: string | null;
	}

	// Each label use is one share of the bar and each unlabeled task is one more,
	// so the bar answers "what is the planned work tagged with", gaps included.
	const segments = $derived.by((): Segment[] => {
		const out: Segment[] = stats.labels.map((u, i) => ({
			key: `label-${u.label.id}`,
			name: u.label.name,
			count: u.count,
			color: u.label.color || FALLBACK_COLORS[i % FALLBACK_COLORS.length]
		}));
		if (stats.unlabeled > 0) {
			out.push({
				key: 'unlabeled',
				name: $t('page.week.planStats.unlabeled'),
				count: stats.unlabeled,
				color: null
			});
		}
		return out;
	});
	const units = $derived(segments.reduce((sum, s) => sum + s.count, 0));
	const legend = $derived(segments.slice(0, LEGEND_LIMIT));
	const hidden = $derived(segments.slice(LEGEND_LIMIT));
	const troikiShare = $derived(stats.total > 0 ? stats.fromTroiki / stats.total : 0);

	function percent(count: number): number {
		return units > 0 ? Math.round((count / units) * 100) : 0;
	}

	function describe(s: Segment): string {
		return `${s.name}: ${s.count} (${percent(s.count)}%)`;
	}
</script>

{#if stats.total > 0}
	<div
		class="mt-3 flex max-w-xl flex-col gap-2"
		data-testid="week-plan-stats"
		aria-label={$t('page.week.planStats.label')}
	>
		<div
			class="flex h-2 w-full overflow-hidden rounded-full bg-muted"
			role="img"
			aria-label={segments.map(describe).join(', ')}
		>
			{#each segments as s (s.key)}
				<div
					class="h-full border-r border-background last:border-r-0 {s.color === null
						? 'bg-muted-foreground/25'
						: ''}"
					style:width={`${(s.count / units) * 100}%`}
					style:background-color={s.color}
					title={describe(s)}
				></div>
			{/each}
		</div>
		<div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
			{#each legend as s (s.key)}
				<span class="inline-flex items-center gap-1.5" title={describe(s)}>
					<span
						class="size-2 shrink-0 rounded-full {s.color === null ? 'bg-muted-foreground/40' : ''}"
						style:background-color={s.color}
					></span>
					<span class={s.color === null ? 'text-muted-foreground italic' : 'text-foreground'}>
						{s.name}
					</span>
					<span class="font-mono text-muted-foreground tabular-nums">{s.count}</span>
				</span>
			{/each}
			{#if hidden.length > 0}
				<span class="text-muted-foreground" title={hidden.map(describe).join('\n')}>
					{$t('page.week.planStats.more', { values: { count: hidden.length } })}
				</span>
			{/if}
			{#if showTroiki}
				<span
					class="inline-flex items-center gap-1.5 sm:ml-auto"
					data-testid="week-plan-stats-troiki"
					title={$t('page.week.planStats.fromTroikiTooltip')}
				>
					<span class="text-muted-foreground">{$t('page.week.planStats.fromTroiki')}</span>
					<span class="h-1.5 w-12 overflow-hidden rounded-full bg-muted">
						<span class="block h-full rounded-full bg-primary" style:width={`${troikiShare * 100}%`}
						></span>
					</span>
					<span class="font-mono text-foreground tabular-nums">
						{stats.fromTroiki}/{stats.total}
					</span>
				</span>
			{/if}
		</div>
	</div>
{/if}
