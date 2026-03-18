<script lang="ts">
	import { pwaUpdate } from '$lib/pwa/update.svelte';
	import { t } from 'svelte-intl-precompile';

	const { needRefresh, updateServiceWorker } = pwaUpdate;
</script>

{#if $needRefresh}
	<div
		class="fixed bottom-4 left-1/2 z-50 flex -translate-x-1/2 items-center gap-3 rounded-lg border border-border bg-background px-4 py-3 shadow-lg"
	>
		<span class="text-sm text-foreground">{$t('pwa.updateAvailable')}</span>
		<button
			class="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90"
			onclick={() => updateServiceWorker(true)}
		>
			{$t('pwa.update')}
		</button>
	</div>
{/if}
