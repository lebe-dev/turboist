<script lang="ts">
	import { t } from '$lib/i18n';

	let {
		title,
		count,
		limit,
		total = null
	}: {
		title: string;
		count: number | null;
		limit: number | null;
		// Total task count including subtasks. Shown as a secondary number when it
		// differs from `count` (which tracks only the limit-bearing top-level tasks).
		total?: number | null;
	} = $props();

	const full = $derived(limit !== null && count !== null && count >= limit);
	const showTotal = $derived(total !== null && total !== count);
</script>

<header class="flex items-center justify-between gap-2 border-b border-border/50 px-3 py-2">
	<h2
		class="text-sm font-semibold uppercase tracking-wide"
		class:text-muted-foreground={!full}
		class:text-red-600={full}
		class:dark:text-red-400={full}
	>
		{title}
	</h2>
	<div class="flex items-center gap-1.5">
		{#if limit !== null && count !== null}
			<span
				class="font-mono text-[11px] tabular-nums"
				class:text-muted-foreground={!full}
				class:text-red-600={full}
				class:dark:text-red-400={full}
				class:font-semibold={full}
			>
				{count} / {limit}
			</span>
		{:else if count !== null}
			<span class="font-mono text-[11px] tabular-nums text-muted-foreground">
				{count}
			</span>
		{/if}
		{#if showTotal}
			<span
				class="font-mono text-[11px] tabular-nums text-muted-foreground/70"
				title={$t('page.nextWeek.withSubtasks', { values: { total: total ?? 0 } })}
			>
				({total})
			</span>
		{/if}
	</div>
</header>
