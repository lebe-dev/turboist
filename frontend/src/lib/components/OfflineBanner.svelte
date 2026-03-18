<script lang="ts">
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import { wsClient } from '$lib/ws/client.svelte';
	import { t } from 'svelte-intl-precompile';
	import WifiOffIcon from '@lucide/svelte/icons/wifi-off';
	import WifiIcon from '@lucide/svelte/icons/wifi';
	import LoaderIcon from '@lucide/svelte/icons/loader';

	let wasOffline = $state(false);
	let showBackOnline = $state(false);
	let dismissTimer: ReturnType<typeof setTimeout> | null = null;

	const isOffline = $derived(tasksStore.isOffline);
	const isReconnecting = $derived(!wsClient.connected && !tasksStore.isOffline);

	$effect(() => {
		if (isOffline) {
			wasOffline = true;
			// Clear any pending "back online" banner
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
		}
	});
</script>

{#if isOffline}
	<div class="flex items-center justify-center gap-2 bg-yellow-500/10 px-3 py-1.5 text-xs text-yellow-600 dark:text-yellow-400">
		<WifiOffIcon class="h-3.5 w-3.5" />
		<span>{$t('pwa.offline')}</span>
	</div>
{:else if isReconnecting}
	<div class="flex items-center justify-center gap-2 bg-amber-500/10 px-3 py-1.5 text-xs text-amber-600 dark:text-amber-400">
		<LoaderIcon class="h-3.5 w-3.5 animate-spin" />
		<span>{$t('connectivity.reconnecting')}</span>
	</div>
{:else if showBackOnline}
	<div class="flex items-center justify-center gap-2 bg-green-500/10 px-3 py-1.5 text-xs text-green-600 dark:text-green-400">
		<WifiIcon class="h-3.5 w-3.5" />
		<span>{$t('connectivity.backOnline')}</span>
	</div>
{/if}
