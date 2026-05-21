<script lang="ts">
	import { parseMarkdownBlocks } from '$lib/utils/markdown';
	import MarkdownInline from './MarkdownInline.svelte';

	let {
		text,
		linkClass = 'text-primary underline-offset-2 hover:underline'
	}: { text: string; linkClass?: string } = $props();

	const blocks = $derived(parseMarkdownBlocks(text));
</script>

{#each blocks as block, i (i)}
	{#if block.type === 'heading'}
		{#if block.level === 1}
			<h1 class="mt-3 mb-1 text-lg font-semibold first:mt-0">
				<MarkdownInline segments={block.segments} {linkClass} />
			</h1>
		{:else if block.level === 2}
			<h2 class="mt-3 mb-1 text-base font-semibold first:mt-0">
				<MarkdownInline segments={block.segments} {linkClass} />
			</h2>
		{:else if block.level === 3}
			<h3 class="mt-2 mb-1 text-sm font-semibold first:mt-0">
				<MarkdownInline segments={block.segments} {linkClass} />
			</h3>
		{:else}
			<h4 class="mt-2 mb-1 text-sm font-medium first:mt-0">
				<MarkdownInline segments={block.segments} {linkClass} />
			</h4>
		{/if}
	{:else if block.type === 'paragraph'}
		<p class="whitespace-pre-wrap break-words [&:not(:first-child)]:mt-2">
			<MarkdownInline segments={block.segments} {linkClass} />
		</p>
	{:else if block.type === 'list'}
		{#if block.ordered}
			<ol class="ml-5 list-decimal [&:not(:first-child)]:mt-2">
				{#each block.items as item, j (j)}
					<li><MarkdownInline segments={item} {linkClass} /></li>
				{/each}
			</ol>
		{:else}
			<ul class="ml-5 list-disc [&:not(:first-child)]:mt-2">
				{#each block.items as item, j (j)}
					<li><MarkdownInline segments={item} {linkClass} /></li>
				{/each}
			</ul>
		{/if}
	{/if}
{/each}
