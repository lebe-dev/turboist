<script lang="ts">
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { wsClient } from '$lib/ws/client.svelte';
	import { t } from 'svelte-intl-precompile';
	import LoaderIcon from '@lucide/svelte/icons/loader';

	let { compact = false }: { compact?: boolean } = $props();

	const isOffline = $derived(tasksStore.isOffline);
	const isReconnecting = $derived(!wsClient.connected && !tasksStore.isOffline);
	const visible = $derived(isOffline || isReconnecting);
</script>

{#if visible}
	{#if compact}
		<!-- Compact mode: small dot for mobile header -->
		<div
			class="relative flex items-center justify-center"
			title={isOffline ? $t('pwa.offline') : $t('connectivity.reconnecting')}
		>
			{#if isOffline}
				<span class="block h-2 w-2 rounded-full bg-yellow-500"></span>
			{:else}
				<span class="block h-2 w-2 animate-pulse rounded-full bg-amber-500"></span>
			{/if}
		</div>
	{:else}
		<!-- Full mode: label for sidebar -->
		{#if isOffline}
			<div class="rounded-md bg-yellow-500/10 px-2 py-0.5 text-[11px] font-medium text-yellow-600 dark:text-yellow-400" title={$t('pwa.offline')}>
				<span class="truncate">Offline</span>
			</div>
		{:else}
			<div class="flex items-center gap-1.5 rounded-md bg-amber-500/10 px-2 py-0.5 text-[11px] font-medium text-amber-600 dark:text-amber-400" title={$t('connectivity.reconnecting')}>
				<LoaderIcon class="h-3 w-3 shrink-0 animate-spin" />
				<span class="truncate">{$t('connectivity.reconnecting')}</span>
			</div>
		{/if}
	{/if}
{/if}
