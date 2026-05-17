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
		countdownSeconds = 0,
		onConfirm
	}: {
		open?: boolean;
		title: string;
		description: string;
		confirmLabel?: string;
		busyLabel?: string;
		variant?: 'destructive' | 'default';
		countdownSeconds?: number;
		onConfirm: () => void | Promise<void>;
	} = $props();

	const resolvedConfirm = $derived(confirmLabel ?? $t('common.delete'));

	let busy = $state(false);
	let remaining = $state(0);

	$effect(() => {
		if (!open || countdownSeconds <= 0) {
			remaining = 0;
			return;
		}
		remaining = countdownSeconds;
		const id = setInterval(() => {
			remaining -= 1;
			if (remaining <= 0) {
				remaining = 0;
				clearInterval(id);
			}
		}, 1000);
		return () => clearInterval(id);
	});

	const locked = $derived(remaining > 0);

	async function confirm() {
		if (busy || locked) return;
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
		<AlertDialog.Footer class={locked ? 'sm:justify-between' : undefined}>
			{#if locked}
				<span
					class="self-center text-sm tabular-nums text-muted-foreground"
					aria-live="polite"
				>
					{$t('common.deleteCountdown', { values: { seconds: remaining } })}
				</span>
			{/if}
			<div class="flex flex-col-reverse gap-2 sm:flex-row">
				<AlertDialog.Cancel disabled={busy}>{$t('common.cancel')}</AlertDialog.Cancel>
				<Button {variant} onclick={confirm} disabled={busy || locked}>
					{busy ? (busyLabel ?? `${resolvedConfirm}…`) : resolvedConfirm}
				</Button>
			</div>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
