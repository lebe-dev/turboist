<script lang="ts">
	import { parseMarkdownLinks } from '$lib/utils/markdown';

	let {
		text,
		linkClass = 'text-primary underline-offset-2 hover:underline'
	}: { text: string; linkClass?: string } = $props();

	const segments = $derived(parseMarkdownLinks(text));
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -->
{#each segments as seg, i (i)}
	{#if seg.type === 'link'}
		<a
			href={seg.href}
			target="_blank"
			rel="noopener noreferrer"
			class={linkClass}
			onclick={(e) => e.stopPropagation()}>{seg.text}</a
		>
	{:else}
		{seg.value}
	{/if}
{/each}
