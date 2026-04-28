<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		subtitle,
		actions,
		banner
	}: {
		title: string;
		subtitle?: string;
		actions?: Snippet;
		banner?: Snippet;
	} = $props();
</script>

<header class="flex flex-col gap-2 border-b border-border px-6 py-4">
	<div class="flex items-start justify-between gap-3">
		<div class="min-w-0">
			<h1 class="truncate text-xl font-semibold">{title}</h1>
			{#if subtitle}
				<p class="text-xs text-muted-foreground">{subtitle}</p>
			{/if}
		</div>
		{#if actions}
			<div class="flex shrink-0 items-center gap-2">{@render actions()}</div>
		{/if}
	</div>
	{#if banner}
		{@render banner()}
	{/if}
</header>
