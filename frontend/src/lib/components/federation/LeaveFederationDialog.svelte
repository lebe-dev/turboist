<script lang="ts">
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/i18n';

	// LeaveFederationDialog asks what to do with the local copy when ending the link
	// with a federated instance (Federation v1 F5.5, US-6.3): keep it as a plain local
	// project, or delete it. Cancel leaves everything untouched. Both actions run with
	// a shared busy guard so the buttons disable while the request is in flight.
	let {
		open = $bindable(false),
		onKeepLocal,
		onDelete
	}: {
		open?: boolean;
		onKeepLocal: () => void | Promise<void>;
		onDelete: () => void | Promise<void>;
	} = $props();

	let busy = $state(false);

	async function run(action: () => void | Promise<void>) {
		if (busy) return;
		busy = true;
		try {
			await action();
			open = false;
		} finally {
			busy = false;
		}
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('federation.leave.confirmTitle')}</AlertDialog.Title>
			<AlertDialog.Description>{$t('federation.leave.confirmBody')}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
				<AlertDialog.Cancel disabled={busy}>{$t('common.cancel')}</AlertDialog.Cancel>
				<Button variant="outline" onclick={() => run(onKeepLocal)} disabled={busy}>
					{$t('federation.leave.keepLocal')}
				</Button>
				<Button variant="destructive" onclick={() => run(onDelete)} disabled={busy}>
					{$t('federation.leave.deleteProject')}
				</Button>
			</div>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
