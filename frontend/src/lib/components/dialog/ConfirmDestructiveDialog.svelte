<script lang="ts">
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/i18n';

	let {
		open = $bindable(false),
		title,
		description,
		confirmLabel,
		busyLabel,
		variant = 'destructive',
		onConfirm
	}: {
		open?: boolean;
		title: string;
		description: string;
		confirmLabel?: string;
		busyLabel?: string;
		variant?: 'destructive' | 'default';
		onConfirm: () => void | Promise<void>;
	} = $props();

	const resolvedConfirm = $derived(confirmLabel ?? $t('common.delete'));

	let busy = $state(false);

	async function confirm() {
		if (busy) return;
		busy = true;
		try {
			await onConfirm();
			open = false;
		} finally {
			busy = false;
		}
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{title}</AlertDialog.Title>
			<AlertDialog.Description>{description}</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={busy}>{$t('common.cancel')}</AlertDialog.Cancel>
			<Button {variant} onclick={confirm} disabled={busy}>
				{busy ? (busyLabel ?? `${resolvedConfirm}…`) : resolvedConfirm}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
