<script lang="ts">
	import { onDestroy } from 'svelte';
	import { toast } from 'svelte-sonner';
	import SparkleIcon from 'phosphor-svelte/lib/Sparkle';
	import { t } from '$lib/i18n';
	import { getApiClient } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { inboxProcessing as inboxProcessingApi } from '$lib/api/endpoints/inbox-processing';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';

	// A manual run ignores the "pause automatic processing" switch: the server
	// only checks it for scheduled runs. The list refreshes on the processor's own
	// SSE events; the status is polled only to report the outcome.
	let { pollIntervalMs = 2000 }: { pollIntervalMs?: number } = $props();

	let busy = $state(false);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let destroyed = false;

	onDestroy(() => {
		destroyed = true;
		if (pollTimer) clearTimeout(pollTimer);
	});

	async function run(): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			await inboxProcessingApi.run(getApiClient());
		} catch (err) {
			if (err instanceof ApiError && err.code === 'inbox_processing_nothing_pending') {
				toast.info($t('page.inbox.sortNothingPending'));
			} else {
				toast.error(describeError(err, $t('settings.inboxProcessing.toasts.runFailed')));
			}
			busy = false;
			return;
		}
		schedulePoll();
	}

	function schedulePoll(): void {
		pollTimer = setTimeout(async () => {
			pollTimer = null;
			if (destroyed) return;
			let next;
			try {
				next = await inboxProcessingApi.status(getApiClient());
			} catch {
				// The run goes on without us; the list still follows the SSE events.
				busy = false;
				return;
			}
			if (destroyed) return;
			if (next.running) {
				schedulePoll();
				return;
			}
			busy = false;
			if (next.lastError) {
				toast.error(next.lastError);
				return;
			}
			if (next.lastRunSummary) {
				toast.success(
					$t('settings.inboxProcessing.toasts.runFinished', { values: { ...next.lastRunSummary } })
				);
			}
		}, pollIntervalMs);
	}
</script>

<Button size="sm" variant="outline" disabled={busy} onclick={run}>
	<SparkleIcon class="size-3.5" weight="fill" />
	{busy ? $t('page.inbox.sorting') : $t('page.inbox.sortWithAi')}
</Button>
