<script lang="ts">
	let { message }: { message: string } = $props();
</script>

<div class="rounded border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
	{message}
</div>
