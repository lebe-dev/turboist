<script lang="ts">
	let {
		count,
		limit
	}: {
		count: number;
		limit: number;
	} = $props();

	const exceeded = $derived(count >= limit);
</script>

<span
	class="inline-flex h-7 items-center rounded-md border px-2.5 font-mono text-[11px] tabular-nums transition-colors"
	class:border-destructive={exceeded}
	class:bg-destructive={exceeded}
	class:text-destructive-foreground={exceeded}
	class:border-border={!exceeded}
	class:bg-muted={!exceeded}
	class:text-muted-foreground={!exceeded}
>
	{count} / {limit}
</span>
