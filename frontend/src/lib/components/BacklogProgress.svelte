<script lang="ts">
	import { t } from 'svelte-intl-precompile';

	let { backlog_count, backlog_limit }: { backlog_count: number; backlog_limit: number } = $props();

	const percent = $derived(Math.min(100, Math.round((backlog_count / backlog_limit) * 100)));
	const isWarning = $derived(percent >= 80 && percent < 100);
	const isOver = $derived(backlog_count >= backlog_limit);
</script>

{#if backlog_limit > 0}
	<div class="border-b border-border/50 px-6 py-3">
		<div class="mb-2 flex items-baseline justify-between">
			<span class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
				{$t('tasks.backlogLabel')}
			</span>
			<span
				class="tabular-nums text-sm font-semibold
					{isOver ? 'text-destructive' : isWarning ? 'text-yellow-500' : 'text-foreground'}"
			>
				{backlog_count}<span class="text-muted-foreground/50">/{backlog_limit}</span>
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
			<p class="mt-1.5 text-[11px] text-destructive/80">{$t('tasks.backlogLimitExceeded')}</p>
		{/if}
	</div>
{/if}
