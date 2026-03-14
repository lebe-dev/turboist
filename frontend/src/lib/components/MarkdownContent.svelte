<script lang="ts">
	type Segment = { type: 'text'; value: string } | { type: 'link'; text: string; href: string };

	let { text, class: className = '' }: { text: string; class?: string } = $props();

	const segments = $derived.by(() => {
		const result: Segment[] = [];
		const re = /\[([^\]]+)\]\((https?:\/\/[^)]+)\)/g;
		let lastIndex = 0;
		let match: RegExpExecArray | null;

		while ((match = re.exec(text)) !== null) {
			if (match.index > lastIndex) {
				result.push({ type: 'text', value: text.slice(lastIndex, match.index) });
			}
			result.push({ type: 'link', text: match[1], href: match[2] });
			lastIndex = re.lastIndex;
		}

		if (lastIndex < text.length) {
			result.push({ type: 'text', value: text.slice(lastIndex) });
		}

		return result;
	});

	const hasLinks = $derived(segments.some((s) => s.type === 'link'));
</script>

{#if hasLinks}
	<span class={className}>{#each segments as seg}{#if seg.type === 'link'}<a
		href={seg.href}
		target="_blank"
		rel="noopener noreferrer"
		class="text-primary underline decoration-primary/30 underline-offset-2 hover:decoration-primary/60"
		onclick={(e) => e.stopPropagation()}
	>{seg.text}</a>{:else}{seg.value}{/if}{/each}</span>
{:else}
	<span class={className}>{text}</span>
{/if}
