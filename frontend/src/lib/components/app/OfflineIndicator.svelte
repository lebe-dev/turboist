<script lang="ts">
	import WifiSlashIcon from 'phosphor-svelte/lib/WifiSlash';
	import { outboxStatusStore } from '$lib/offline/outboxStatus.svelte';
	import { t } from '$lib/i18n';
</script>

{#if !outboxStatusStore.online}
	<span
		class="inline-flex h-6 shrink-0 items-center gap-1.5 rounded-md border border-amber-500/40 bg-amber-500/10 px-2 text-[12px] text-amber-700 dark:text-amber-300"
		role="status"
		aria-live="polite"
		data-testid="offline-indicator"
		title={$t('offline.indicator.title')}
	>
		<WifiSlashIcon class="size-3" />
		<span>{$t('offline.indicator.label')}</span>
	</span>
{/if}
