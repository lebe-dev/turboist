<script lang="ts">
	import type { InlineSegment } from '$lib/utils/markdown';
	import Self from './MarkdownInline.svelte';

	let {
		segments,
		linkClass = 'text-primary underline-offset-2 hover:underline'
	}: { segments: InlineSegment[]; linkClass?: string } = $props();
</script>

<!-- eslint-disable svelte/no-navigation-without-resolve -->
{#each segments as seg, i (i)}
	{#if seg.type === 'text'}{seg.value}{:else if seg.type === 'code'}<code
			class="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">{seg.value}</code
		>{:else if seg.type === 'bold'}<strong class="font-semibold"
			><Self segments={seg.segments} {linkClass} /></strong
		>{:else if seg.type === 'italic'}<em><Self segments={seg.segments} {linkClass} /></em
		>{:else if seg.type === 'link'}<a
			href={seg.href}
			target="_blank"
			rel="noopener noreferrer"
			class={linkClass}
			onclick={(e) => e.stopPropagation()}>{seg.text}</a
		>{/if}
{/each}
