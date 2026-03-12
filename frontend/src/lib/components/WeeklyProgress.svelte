<script lang="ts">
	let { weekly_count, weekly_limit }: { weekly_count: number; weekly_limit: number } = $props();

	const percent = $derived(Math.min(100, Math.round((weekly_count / weekly_limit) * 100)));
	const isWarning = $derived(percent >= 80 && percent < 100);
	const isOver = $derived(weekly_count >= weekly_limit);
</script>

{#if weekly_limit > 0}
	<div class="border-b border-border/50 px-6 py-3">
		<div class="mb-2 flex items-baseline justify-between">
			<span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
				На неделе
			</span>
			<span
				class="tabular-nums text-sm font-semibold
					{isOver ? 'text-destructive' : isWarning ? 'text-yellow-500' : 'text-foreground'}"
			>
				{weekly_count}<span class="text-muted-foreground/50">/{weekly_limit}</span>
			</span>
		</div>
		<div class="h-1 overflow-hidden rounded-full bg-muted">
			<div
				class="h-full rounded-full transition-all duration-500 ease-out
					{isOver ? 'bg-destructive' : isWarning ? 'bg-yellow-500' : 'bg-primary'}"
				style="width: {percent}%"
			></div>
		</div>
		{#if isOver}
			<p class="mt-1.5 text-[11px] text-destructive/80">Лимит задач на неделю превышен</p>
		{/if}
	</div>
{/if}
