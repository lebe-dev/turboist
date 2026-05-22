<script lang="ts">
	let { label, class: cls = '' }: { label: string; class?: string } = $props();
</script>

<h2 class="px-3 pb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground {cls}">
	{label}
</h2>
