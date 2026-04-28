<script lang="ts">
	import type { Label } from '$lib/api/types';

	let { labels }: { labels: Label[] } = $props();
</script>

{#if labels.length > 0}
	<div class="flex flex-wrap items-center gap-1">
		{#each labels as label (label.id)}
			<span
				class="inline-flex items-center rounded-full bg-accent/70 px-2 py-0.5 text-[11px] font-medium text-accent-foreground"
				style={label.color ? `--label-dot: ${label.color};` : ''}
			>
				{#if label.color}
					<span
						class="mr-1 inline-block size-1.5 rounded-full"
						style={`background-color: ${label.color}`}
					></span>
				{/if}
				{label.name}
			</span>
		{/each}
	</div>
{/if}
