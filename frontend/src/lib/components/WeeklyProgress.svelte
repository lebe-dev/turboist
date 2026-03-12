<script lang="ts">
	let { weekly_count, weekly_limit }: { weekly_count: number; weekly_limit: number } = $props();

	const percent = $derived(Math.min(100, Math.round((weekly_count / weekly_limit) * 100)));
	const isWarning = $derived(percent >= 80 && percent < 100);
	const isOver = $derived(weekly_count >= weekly_limit);
</script>

{#if weekly_limit > 0}
	<div class="border-b border-border px-4 py-3">
		<div class="mb-1.5 flex items-center justify-between text-sm">
			<span class="font-medium text-foreground">На неделе</span>
			<span
				class:text-yellow-500={isWarning}
				class:text-destructive={isOver}
				class="tabular-nums text-muted-foreground"
			>
				{weekly_count} / {weekly_limit}
			</span>
		</div>
		<div class="h-1.5 overflow-hidden rounded-full bg-muted">
			<div
				class="h-full rounded-full transition-all duration-300"
				class:bg-primary={!isWarning && !isOver}
				class:bg-yellow-500={isWarning}
				class:bg-destructive={isOver}
				style="width: {percent}%"
			></div>
		</div>
		{#if isOver}
			<p class="mt-1.5 text-xs text-destructive">Лимит задач на неделю превышен</p>
		{/if}
	</div>
{/if}
