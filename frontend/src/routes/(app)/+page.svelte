<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';

	onMount(() => {
		void goto(resolve('/today'), { replaceState: true });
	});
</script>

<div class="p-6 text-sm text-muted-foreground">{$t('app.redirecting')}</div>
