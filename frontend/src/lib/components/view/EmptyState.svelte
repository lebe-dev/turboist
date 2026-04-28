<script lang="ts">
	import type { Component } from 'svelte';

	let {
		title,
		description,
		icon: Icon
	}: {
		title: string;
		description?: string;
		icon?: Component<{ class?: string }>;
	} = $props();
</script>

<div class="flex flex-col items-center justify-center gap-2 px-6 py-16 text-center">
	{#if Icon}
		<Icon class="size-10 text-muted-foreground/50" />
	{/if}
	<p class="text-sm font-medium">{title}</p>
	{#if description}
		<p class="max-w-sm text-xs text-muted-foreground">{description}</p>
	{/if}
</div>
