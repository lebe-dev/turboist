<script lang="ts">
	import type { Label } from '$lib/api/types';
	import { Badge } from '$lib/components/ui/badge';

	let { labels }: { labels: Label[] } = $props();
</script>

{#if labels.length > 0}
	<div class="flex flex-wrap gap-1">
		{#each labels as label (label.id)}
			<Badge variant="outline" class="h-4 text-[10px]">@{label.name}</Badge>
		{/each}
	</div>
{/if}
