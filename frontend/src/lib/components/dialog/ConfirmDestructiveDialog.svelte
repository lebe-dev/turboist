<script lang="ts">
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';

	let {
		open = $bindable(false),
		title,
		description,
		confirmLabel = 'Delete',
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
			<AlertDialog.Cancel disabled={busy}>Cancel</AlertDialog.Cancel>
			<Button {variant} onclick={confirm} disabled={busy}>
				{busy ? (busyLabel ?? `${confirmLabel}…`) : confirmLabel}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
