<script lang="ts">
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { t } from 'svelte-intl-precompile';
</script>

{#if tasksStore.isOffline}
	<div class="flex items-center justify-center gap-2 bg-yellow-500/10 px-3 py-1.5 text-xs text-yellow-600 dark:text-yellow-400">
		<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
			<line x1="2" y1="2" x2="22" y2="22" />
			<path d="M8.5 16.5a5 5 0 0 1 7 0" />
			<path d="M2 8.82a15 15 0 0 1 4.17-2.65" />
			<path d="M10.66 5c4.01-.36 8.14.9 11.34 3.76" />
			<path d="M16.85 11.25a10 10 0 0 1 2.22 1.68" />
			<path d="M5 12.86a10 10 0 0 1 5.17-2.89" />
			<line x1="12" y1="20" x2="12.01" y2="20" />
		</svg>
		<span>{$t('pwa.offline')}</span>
	</div>
{/if}
