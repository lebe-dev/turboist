<script lang="ts">
	let {
		title,
		count,
		limit
	}: {
		title: string;
		count: number | null;
		limit: number | null;
	} = $props();

	const full = $derived(limit !== null && count !== null && count >= limit);
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
</header>
