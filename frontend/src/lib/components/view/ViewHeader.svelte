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

<header class="flex flex-col gap-3 px-4 pt-5 pb-4 sm:px-8 sm:pt-8">
	<div class="flex items-end justify-between gap-3">
		<div class="min-w-0 space-y-1">
			<h1 class="truncate text-xl font-semibold leading-tight tracking-tight text-foreground sm:text-[26px]">
				{title}
			</h1>
			{#if subtitle}
				<p class="text-sm text-muted-foreground">{subtitle}</p>
			{/if}
		</div>
		{#if actions}
			<div class="flex shrink-0 items-center gap-2">{@render actions()}</div>
		{/if}
	</div>
	{#if banner}
		<div class="-mx-1">
			{@render banner()}
		</div>
	{/if}
</header>
