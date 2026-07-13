<script lang="ts">
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwise';
	import { toast } from 'svelte-sonner';
	import { t } from '$lib/i18n';
	import { calendarReauthStore } from '$lib/stores/calendarReauth.svelte';
	import { calendars as calendarsApi } from '$lib/api/endpoints/calendars';
	import { getApiClient } from '$lib/api/client';
	import { describeError } from '$lib/utils/taskActions';

	let busy = $state(false);

	async function reconnect(): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			const res = await calendarsApi.googleStart(getApiClient());
			window.location.href = res.url;
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.connectFailed')));
			busy = false;
		}
	}
</script>

{#if calendarReauthStore.needed}
	<div
		class="flex items-center gap-3 border-b border-amber-500/30 bg-amber-500/10 py-2 pl-3 pr-4 text-sm text-foreground sm:pl-4 sm:pr-6"
		role="alert"
	>
		<WarningIcon class="size-4 shrink-0 text-amber-600 dark:text-amber-400" weight="fill" />
		<span class="min-w-0 flex-1">{$t('settings.calendars.reauthBanner')}</span>
		<button
			type="button"
			onclick={reconnect}
			disabled={busy}
			class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-amber-500/40 bg-background/60 px-2.5 py-1 text-xs font-medium transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
		>
			<ArrowsClockwiseIcon class="size-3.5" />
			{$t('settings.calendars.reconnect')}
		</button>
	</div>
{/if}
