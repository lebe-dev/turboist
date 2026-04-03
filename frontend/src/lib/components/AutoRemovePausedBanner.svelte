<script lang="ts">
	import { appStore } from '$lib/stores/app.svelte';
	import { t } from 'svelte-intl-precompile';
	import FlameIcon from '@lucide/svelte/icons/flame';

	let dismissed = $state(false);
</script>

{#if appStore.autoRemovePaused && !dismissed}
	<div
		class="fixed bottom-4 left-4 right-4 z-50 flex items-center gap-3 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 shadow-lg sm:left-1/2 sm:right-auto sm:-translate-x-1/2"
	>
		<FlameIcon class="h-4 w-4 shrink-0 text-destructive" />
		<span class="text-sm text-foreground">{$t('tasks.autoRemovePaused')}</span>
		<button
			class="ml-auto shrink-0 text-muted-foreground hover:text-foreground"
			onclick={() => (dismissed = true)}
			aria-label="Dismiss"
		>
			<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
				<line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
			</svg>
		</button>
	</div>
{/if}
