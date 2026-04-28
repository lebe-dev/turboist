<script lang="ts">
	import { Badge } from '$lib/components/ui/badge';

	let {
		count,
		limit
	}: {
		count: number;
		limit: number;
	} = $props();

	const exceeded = $derived(count >= limit);
</script>

<Badge variant={exceeded ? 'destructive' : 'secondary'} class="h-5 text-[11px]">
	{count} / {limit}
</Badge>
