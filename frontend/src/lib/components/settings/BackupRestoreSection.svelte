<script lang="ts">
	import { toast } from 'svelte-sonner';
	import DownloadIcon from 'phosphor-svelte/lib/DownloadSimple';
	import UploadIcon from 'phosphor-svelte/lib/UploadSimple';
	import { t } from '$lib/i18n';
	import { backup } from '$lib/api';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';

	const RESTORE_COUNTDOWN_SECONDS = 20;

	let includeSettings = $state(true);
	let downloading = $state(false);
	let selectedFile = $state<File | null>(null);
	let confirmOpen = $state(false);
	let countdown = $state(RESTORE_COUNTDOWN_SECONDS);
	let restoring = $state(false);
	let fileInput: HTMLInputElement | null = $state(null);
	let timer: ReturnType<typeof setInterval> | null = null;

	async function onDownload() {
		if (downloading) return;
		downloading = true;
		try {
			const { blob, filename } = await backup.download(includeSettings);
			triggerBlobDownload(blob, filename);
		} catch (err) {
			toast.error(describeError(err, $t('settings.backup.downloadFailed')));
		} finally {
			downloading = false;
		}
	}

	function triggerBlobDownload(blob: Blob, filename: string) {
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = filename;
		document.body.appendChild(a);
		a.click();
		a.remove();
		URL.revokeObjectURL(url);
	}

	function onFileChange(event: Event) {
		const input = event.currentTarget as HTMLInputElement;
		selectedFile = input.files?.[0] ?? null;
	}

	function openConfirm() {
		if (!selectedFile) return;
		countdown = RESTORE_COUNTDOWN_SECONDS;
		confirmOpen = true;
	}

	function startCountdown() {
		stopCountdown();
		countdown = RESTORE_COUNTDOWN_SECONDS;
		timer = setInterval(() => {
			countdown -= 1;
			if (countdown <= 0) {
				stopCountdown();
				countdown = 0;
			}
		}, 1000);
	}

	function stopCountdown() {
		if (timer !== null) {
			clearInterval(timer);
			timer = null;
		}
	}

	$effect(() => {
		if (confirmOpen) {
			startCountdown();
		} else {
			stopCountdown();
		}
		return () => stopCountdown();
	});

	async function onConfirmRestore() {
		if (!selectedFile || countdown > 0 || restoring) return;
		restoring = true;
		try {
			await backup.restore(selectedFile);
			toast.success($t('settings.backup.restoreSuccess'));
			confirmOpen = false;
			selectedFile = null;
			if (fileInput) fileInput.value = '';
			// Reload so all stores reflect the new dataset rather than the old in-memory state.
			window.location.reload();
		} catch (err) {
			toast.error(describeError(err, $t('settings.backup.restoreFailed')));
		} finally {
			restoring = false;
		}
	}
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex flex-col gap-0.5">
		<h2 class="text-sm font-semibold">{$t('settings.backup.heading')}</h2>
		<p class="text-xs text-muted-foreground">{$t('settings.backup.description')}</p>
	</div>

	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<span class="text-sm font-medium">{$t('settings.backup.includeSettings')}</span>
			<p class="text-xs text-muted-foreground">{$t('settings.backup.includeSettingsHint')}</p>
		</div>
		<Switch
			checked={includeSettings}
			onCheckedChange={(v) => (includeSettings = v)}
			aria-label={$t('settings.backup.includeSettings')}
		/>
	</div>

	<div>
		<Button type="button" variant="secondary" onclick={onDownload} disabled={downloading}>
			<DownloadIcon class="size-4" />
			{downloading ? $t('settings.backup.downloading') : $t('settings.backup.download')}
		</Button>
	</div>
</section>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex flex-col gap-0.5">
		<h2 class="text-sm font-semibold">{$t('settings.backup.restoreHeading')}</h2>
		<p class="text-xs text-muted-foreground">{$t('settings.backup.restoreDescription')}</p>
	</div>

	<div class="flex flex-col gap-2 sm:flex-row sm:items-center">
		<label
			class="inline-flex cursor-pointer items-center gap-2 rounded-md border border-input bg-background px-3 py-1.5 text-xs font-medium shadow-sm transition-colors hover:bg-muted focus-within:outline-none focus-within:ring-[3px] focus-within:ring-ring/50"
		>
			<UploadIcon class="size-4" />
			{$t('settings.backup.chooseFile')}
			<input
				bind:this={fileInput}
				type="file"
				accept="application/json,.json,.gz"
				class="sr-only"
				onchange={onFileChange}
			/>
		</label>
		<span class="truncate text-xs text-muted-foreground">
			{selectedFile?.name ?? $t('settings.backup.noFile')}
		</span>
		<div class="sm:ml-auto">
			<Button
				type="button"
				variant="destructive"
				disabled={!selectedFile}
				onclick={openConfirm}
			>
				{$t('settings.backup.restore')}
			</Button>
		</div>
	</div>
</section>

<AlertDialog.Root bind:open={confirmOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('settings.backup.confirmTitle')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('settings.backup.confirmDescription', {
					values: {
						name: selectedFile?.name ?? '',
						seconds: countdown
					}
				})}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={restoring}>
				{$t('settings.backup.cancel')}
			</AlertDialog.Cancel>
			<AlertDialog.Action
				onclick={onConfirmRestore}
				disabled={countdown > 0 || restoring}
			>
				{restoring
					? $t('settings.backup.restoring')
					: countdown > 0
						? `${$t('settings.backup.restore')} (${countdown})`
						: $t('settings.backup.restore')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
