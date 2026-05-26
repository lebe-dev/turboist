<script lang="ts">
	import CloudSlashIcon from 'phosphor-svelte/lib/CloudSlash';
	import { t } from '$lib/i18n';

	let { visible }: { visible: boolean } = $props();
</script>

{#if visible}
	<div
		class="flex items-center gap-2 border-b border-amber-500/30 bg-amber-500/10 px-4 py-1.5 text-[12px] text-amber-700 dark:text-amber-300"
		role="status"
		aria-live="polite"
		data-testid="stale-data-banner"
	>
		<CloudSlashIcon class="size-3.5" />
		<span>{$t('offline.staleData')}</span>
	</div>
{/if}
