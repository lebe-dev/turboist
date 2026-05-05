<script lang="ts">
	import type { Component, Snippet } from 'svelte';
	import EmptyState from './EmptyState.svelte';

	let {
		loading,
		isEmpty,
		emptyIcon,
		emptyTitle,
		emptyDescription,
		children
	}: {
		loading: boolean;
		isEmpty: boolean;
		emptyIcon?: Component<{ class?: string }>;
		emptyTitle: string;
		emptyDescription?: string;
		children: Snippet;
	} = $props();
</script>

{#if loading}
	<div class="px-4 py-8 text-sm text-muted-foreground">Loading…</div>
{:else if isEmpty}
	<EmptyState icon={emptyIcon} title={emptyTitle} description={emptyDescription} />
{:else}
	{@render children()}
{/if}
