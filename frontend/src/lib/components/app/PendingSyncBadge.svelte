<script lang="ts">
	import CloudArrowUpIcon from 'phosphor-svelte/lib/CloudArrowUp';
	import { outboxStatusStore } from '$lib/offline/outboxStatus.svelte';
	import { t } from '$lib/i18n';
</script>

{#if outboxStatusStore.pending > 0}
	<span
		class="inline-flex h-6 shrink-0 items-center gap-1.5 rounded-md border border-border bg-muted px-2 text-[12px] text-muted-foreground"
		role="status"
		aria-live="polite"
		data-testid="pending-sync-badge"
		title={$t('offline.pending.title', { values: { count: outboxStatusStore.pending } })}
	>
		<CloudArrowUpIcon class="size-3" />
		<span>{outboxStatusStore.pending}</span>
	</span>
{/if}
