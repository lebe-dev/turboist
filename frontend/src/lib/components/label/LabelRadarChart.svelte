<script lang="ts">
	import { resolve } from '$app/paths';
	import { radarPoint, type LabelRadar } from '$lib/utils/labelStats';
	import { t } from '$lib/i18n';

	let { radar }: { radar: LabelRadar } = $props();

	const RADIUS = 100;
	const LABEL_GAP = 14;
	const RINGS = [0.25, 0.5, 0.75, 1];

	function polygon(fractionOf: (i: number) => number): string {
		return radar.axes
			.map((axis, i) => radarPoint(axis.angle, fractionOf(i), RADIUS).join(','))
			.join(' ');
	}

	const currentShape = $derived(polygon((i) => radar.axes[i].current));
	const previousShape = $derived(polygon((i) => radar.axes[i].previous));

	// Names sit just past the rim; their anchor follows the side of the circle so
	// the text grows away from the chart instead of across it.
	function anchor(angle: number): 'start' | 'middle' | 'end' {
		const x = Math.sin(angle);
		if (Math.abs(x) < 0.2) return 'middle';
		return x > 0 ? 'start' : 'end';
	}

	function baseline(angle: number): 'auto' | 'middle' | 'hanging' {
		const y = -Math.cos(angle);
		if (Math.abs(y) < 0.3) return 'middle';
		return y > 0 ? 'hanging' : 'auto';
	}

	function axisTitle(axis: LabelRadar['axes'][number]): string {
		return `${axis.row.item.label.name}: ${axis.row.applied} · ${$t('page.labels.trendTooltip', {
			values: { previous: axis.row.previousApplied }
		})}`;
	}
</script>

<figure class="flex flex-col items-center gap-2">
	<svg
		viewBox="-190 -140 380 280"
		class="w-full max-w-xl overflow-visible"
		role="img"
		aria-label={$t('page.labels.radar.aria')}
	>
		<g class="text-border" fill="none" stroke="currentColor" stroke-width="1">
			{#each RINGS as ring (ring)}
				<polygon points={polygon(() => ring)} opacity={ring === 1 ? 1 : 0.6} />
			{/each}
			{#each radar.axes as axis (axis.row.item.label.id)}
				{@const [x, y] = radarPoint(axis.angle, 1, RADIUS)}
				<line x1="0" y1="0" x2={x} y2={y} opacity="0.6" />
			{/each}
		</g>

		<polygon
			points={previousShape}
			class="text-muted-foreground"
			fill="none"
			stroke="currentColor"
			stroke-width="1.5"
			stroke-dasharray="4 3"
			stroke-linejoin="round"
		/>
		<polygon
			points={currentShape}
			class="text-primary"
			fill="currentColor"
			fill-opacity="0.18"
			stroke="currentColor"
			stroke-width="2"
			stroke-linejoin="round"
		/>

		{#each radar.axes as axis (axis.row.item.label.id)}
			{@const [px, py] = radarPoint(axis.angle, axis.current, RADIUS)}
			{@const [lx, ly] = radarPoint(axis.angle, 1, RADIUS + LABEL_GAP)}
			<g>
				<title>{axisTitle(axis)}</title>
				<circle
					cx={px}
					cy={py}
					r="3.5"
					fill={axis.row.item.label.color}
					class="stroke-card"
					stroke-width="1.5"
				/>
				<a
					href={resolve('/(app)/label/[id]', { id: String(axis.row.item.label.id) })}
					aria-hidden="true"
					tabindex="-1"
				>
					<text
						x={lx}
						y={ly}
						text-anchor={anchor(axis.angle)}
						dominant-baseline={baseline(axis.angle)}
						class="fill-muted-foreground text-[11px] hover:fill-foreground"
						data-radar-axis
						>{axis.row.item.label.name}<tspan class="fill-foreground font-semibold" dx="4"
							>{axis.row.applied}</tspan
						></text
					>
				</a>
			</g>
		{/each}
	</svg>
	<figcaption
		class="flex flex-wrap items-center justify-center gap-x-4 gap-y-1 text-xs text-muted-foreground"
	>
		<span class="flex items-center gap-1.5">
			<span class="inline-block h-2.5 w-4 rounded-sm border-2 border-primary bg-primary/20"></span>
			{$t('page.labels.radar.current')}
		</span>
		<span class="flex items-center gap-1.5">
			<span class="inline-block h-0 w-4 border-t-2 border-dashed border-muted-foreground"></span>
			{$t('page.labels.radar.previous')}
		</span>
		{#if radar.hidden > 0}
			<span>{$t('page.labels.radar.hidden', { values: { count: radar.hidden } })}</span>
		{/if}
	</figcaption>
</figure>
