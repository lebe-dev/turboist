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

<div class="flex flex-col items-center justify-center gap-3 px-6 py-24 text-center">
	{#if Icon}
		<div
			class="flex size-14 items-center justify-center rounded-2xl border border-border/60 bg-muted/30 text-muted-foreground/70"
		>
			<Icon class="size-7" />
		</div>
	{/if}
	<p class="text-base font-medium text-foreground">{title}</p>
	{#if description}
		<p class="max-w-sm text-sm text-muted-foreground">{description}</p>
	{/if}
</div>
