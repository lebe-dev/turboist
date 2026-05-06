<script lang="ts">
	import ClockClockwiseIcon from 'phosphor-svelte/lib/ClockClockwise';
	import { t } from '$lib/i18n';

	let { count, completed = false }: { count: number; completed?: boolean } = $props();

	const spanClass = $derived.by(() => {
		const base = 'inline-flex items-center gap-1 text-xs font-medium';
		if (completed) {
			const hover = count > 2
				? 'group-hover/task:text-red-600'
				: 'group-hover/task:text-amber-600';
			return `${base} text-muted-foreground ${hover}`;
		}
		return `${base} ${count === 2 ? 'text-amber-600' : 'text-red-600'}`;
	});
</script>

{#if count >= 2}
	<span class={spanClass} title={$t('task.postponedTimes', { values: { count } })}>
		<ClockClockwiseIcon class="size-3.5 shrink-0" />
		<span>{count}</span>
	</span>
{/if}
