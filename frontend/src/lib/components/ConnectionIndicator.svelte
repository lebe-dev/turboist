<script lang="ts">
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { wsClient } from '$lib/ws/client.svelte';
	import { t } from 'svelte-intl-precompile';
	import WifiOffIcon from '@lucide/svelte/icons/wifi-off';
	import WifiIcon from '@lucide/svelte/icons/wifi';
	import LoaderIcon from '@lucide/svelte/icons/loader';

	let { compact = false }: { compact?: boolean } = $props();

	let wasOffline = $state(false);
	let showBackOnline = $state(false);
	let dismissTimer: ReturnType<typeof setTimeout> | null = null;

	const isOffline = $derived(tasksStore.isOffline);
	const isReconnecting = $derived(!wsClient.connected && !tasksStore.isOffline);

	$effect(() => {
		if (isOffline) {
			wasOffline = true;
			if (dismissTimer) {
				clearTimeout(dismissTimer);
				dismissTimer = null;
			}
			showBackOnline = false;
		}
	});

	$effect(() => {
		if (wasOffline && wsClient.connected && !isOffline) {
			showBackOnline = true;
			wasOffline = false;
			dismissTimer = setTimeout(() => {
				showBackOnline = false;
				dismissTimer = null;
			}, 3000);
			return () => {
				if (dismissTimer) {
					clearTimeout(dismissTimer);
					dismissTimer = null;
				}
			};
		}
	});

	const visible = $derived(isOffline || isReconnecting || showBackOnline);
</script>

{#if visible}
	{#if compact}
		<!-- Compact mode: colored dot with tooltip, for mobile header -->
		<div
			class="relative flex items-center justify-center"
			title={isOffline
				? $t('pwa.offline')
				: isReconnecting
					? $t('connectivity.reconnecting')
					: $t('connectivity.backOnline')}
		>
			{#if isOffline}
				<span class="block h-2.5 w-2.5 rounded-full bg-yellow-500"></span>
			{:else if isReconnecting}
				<span class="block h-2.5 w-2.5 animate-pulse rounded-full bg-amber-500"></span>
			{:else}
				<span class="block h-2.5 w-2.5 rounded-full bg-green-500 transition-opacity duration-1000"></span>
			{/if}
		</div>
	{:else}
		<!-- Full mode: icon + label, for sidebar -->
		{#if isOffline}
			<div class="flex items-center gap-1.5 rounded-md bg-yellow-500/10 px-2 py-0.5 text-[11px] font-medium text-yellow-600 dark:text-yellow-400" title={$t('pwa.offline')}>
				<WifiOffIcon class="h-3 w-3 shrink-0" />
				<span class="truncate">Offline</span>
			</div>
		{:else if isReconnecting}
			<div class="flex items-center gap-1.5 rounded-md bg-amber-500/10 px-2 py-0.5 text-[11px] font-medium text-amber-600 dark:text-amber-400" title={$t('connectivity.reconnecting')}>
				<LoaderIcon class="h-3 w-3 shrink-0 animate-spin" />
				<span class="truncate">{$t('connectivity.reconnecting')}</span>
			</div>
		{:else if showBackOnline}
			<div class="flex items-center gap-1.5 rounded-md bg-green-500/10 px-2 py-0.5 text-[11px] font-medium text-green-600 dark:text-green-400" title={$t('connectivity.backOnline')}>
				<WifiIcon class="h-3 w-3 shrink-0" />
				<span class="truncate">{$t('connectivity.backOnline')}</span>
			</div>
		{/if}
	{/if}
{/if}
