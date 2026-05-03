<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';

	onMount(() => {
		void goto(resolve('/today'), { replaceState: true });
	});
</script>

<div class="p-6 text-sm text-muted-foreground">Redirecting…</div>
