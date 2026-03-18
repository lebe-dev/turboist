<script lang="ts">
	import { pwaUpdate } from '$lib/pwa/update.svelte';
	import { t } from 'svelte-intl-precompile';

	const { needRefresh, updateServiceWorker } = pwaUpdate;

	let dismissed = $state(false);
</script>

{#if $needRefresh && !dismissed}
	<div
		class="fixed bottom-4 left-4 right-4 z-50 flex items-center gap-3 rounded-lg border border-border bg-background px-4 py-3 shadow-lg sm:left-1/2 sm:right-auto sm:-translate-x-1/2"
	>
		<span class="text-sm text-foreground">{$t('pwa.updateAvailable')}</span>
		<button
			class="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90"
			onclick={() => updateServiceWorker(true)}
		>
			{$t('pwa.update')}
		</button>
		<button
			class="ml-auto text-muted-foreground hover:text-foreground"
			onclick={() => (dismissed = true)}
			aria-label="Dismiss"
		>
			<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
			</svg>
		</button>
	</div>
{/if}
