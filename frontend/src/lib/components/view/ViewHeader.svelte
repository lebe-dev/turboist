<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		actions,
		banner
	}: {
		title?: string;
		actions?: Snippet;
		banner?: Snippet;
	} = $props();
</script>

{#if title || actions || banner}
<header class="flex flex-col gap-3 px-4 pt-4 pb-3 sm:px-8">
	{#if title || actions}
		<div class="flex items-center justify-between gap-3">
			{#if title}
				<h1 class="truncate text-xl font-semibold leading-tight tracking-tight text-foreground sm:text-[26px]">
					{title}
				</h1>
			{/if}
			{#if actions}
				<div class="flex shrink-0 items-center gap-2">{@render actions()}</div>
			{/if}
		</div>
	{/if}
	{#if banner}
		<div class="-mx-1">
			{@render banner()}
		</div>
	{/if}
</header>
{/if}
