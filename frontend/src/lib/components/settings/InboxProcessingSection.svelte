<script lang="ts">
	import { onDestroy, onMount, untrack } from 'svelte';
	import { toast } from 'svelte-sonner';
	import PlayIcon from 'phosphor-svelte/lib/Play';
	import EyeIcon from 'phosphor-svelte/lib/Eye';
	import ArrowCounterClockwiseIcon from 'phosphor-svelte/lib/ArrowCounterClockwise';
	import SparkleIcon from 'phosphor-svelte/lib/Sparkle';
	import { t, locale } from '$lib/i18n';
	import { getApiClient } from '$lib/api/client';
	import { ApiError } from '$lib/api/errors';
	import { inboxProcessing as inboxProcessingApi } from '$lib/api/endpoints/inbox-processing';
	import type { InboxProcessingStatus } from '$lib/api/types';
	import { appSettingsStore } from '$lib/stores/appSettings.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { useInvalidation } from '$lib/hooks/useInvalidation.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import { Textarea } from '$lib/components/ui/textarea';
	import { Switch } from '$lib/components/ui/switch';
	import * as Sheet from '$lib/components/ui/sheet';
	import InboxProcessingLog from './InboxProcessingLog.svelte';

	// How often the status is re-read while a manual run is in flight.
	let { pollIntervalMs = 2000 }: { pollIntervalMs?: number } = $props();

	let status = $state<InboxProcessingStatus | null>(null);
	let statusError = $state(false);
	let starting = $state(false);
	let polling = $state(false);
	let saving = $state(false);
	let previewing = $state(false);
	let templateError = $state<string | null>(null);
	let previewOpen = $state(false);
	let previewText = $state('');
	let logRefreshKey = $state(0);
	let pollTimer: ReturnType<typeof setTimeout> | null = null;
	let destroyed = false;

	const savedPrompt = $derived(appSettingsStore.inboxProcessing.prompt);
	const paused = $derived(appSettingsStore.inboxProcessing.paused);
	let pauseBusy = $state(false);

	async function setPaused(next: boolean): Promise<void> {
		if (pauseBusy) return;
		pauseBusy = true;
		try {
			await appSettingsStore.setInboxProcessingPaused(next);
			toast.success(
				next
					? $t('settings.inboxProcessing.toasts.paused')
					: $t('settings.inboxProcessing.toasts.resumed')
			);
		} catch (err) {
			toast.error(describeError(err, $t('settings.inboxProcessing.toasts.pauseFailed')));
		} finally {
			pauseBusy = false;
		}
	}

	const defaultPrompt = $derived(status?.defaultPrompt ?? '');
	// What the editor should show for the stored value: an empty stored prompt
	// means the built-in default, so the default text is what the user edits.
	const effectiveSaved = $derived(savedPrompt || defaultPrompt);
	const usingDefault = $derived(savedPrompt === '');

	let draft = $state('');
	let lastSeen = '';
	$effect(() => {
		const next = effectiveSaved;
		untrack(() => {
			// Follow the stored value (and the default arriving with the status)
			// without wiping text the user is still editing.
			if (draft === lastSeen) draft = next;
			lastSeen = next;
		});
	});
	const dirty = $derived(draft !== effectiveSaved);

	const running = $derived(starting || polling || (status?.running ?? false));
	const canRun = $derived(!!status?.enabled && !running && (status?.pendingCount ?? 0) > 0);

	async function loadStatus(): Promise<InboxProcessingStatus | null> {
		try {
			const next = await inboxProcessingApi.status(getApiClient());
			status = next;
			statusError = false;
			return next;
		} catch {
			statusError = true;
			return null;
		}
	}

	onMount(() => {
		void loadStatus();
	});

	onDestroy(() => {
		destroyed = true;
		if (pollTimer) clearTimeout(pollTimer);
	});

	// Keep the waiting counter honest while tasks arrive in or leave the Inbox.
	useInvalidation(['inbox', 'tasks'], () => {
		if (!polling) void loadStatus();
	});

	async function runNow(): Promise<void> {
		if (!canRun) return;
		starting = true;
		try {
			await inboxProcessingApi.run(getApiClient());
		} catch (err) {
			if (err instanceof ApiError && err.code === 'inbox_processing_nothing_pending') {
				toast.info($t('settings.inboxProcessing.runDisabledHint'));
				void loadStatus();
			} else {
				toast.error(describeError(err, $t('settings.inboxProcessing.toasts.runFailed')));
			}
			starting = false;
			return;
		}
		starting = false;
		polling = true;
		schedulePoll();
	}

	function schedulePoll(): void {
		pollTimer = setTimeout(async () => {
			pollTimer = null;
			if (destroyed) return;
			const next = await loadStatus();
			if (destroyed) return;
			if (next?.running) {
				schedulePoll();
				return;
			}
			polling = false;
			logRefreshKey += 1;
			const summary = next?.lastRunSummary;
			if (next?.lastError) {
				toast.error(next.lastError);
			} else if (summary) {
				toast.success($t('settings.inboxProcessing.toasts.runFinished', { values: { ...summary } }));
			}
		}, pollIntervalMs);
	}

	function templateErrorFrom(err: unknown): string | null {
		if (err instanceof ApiError && err.status === 422) {
			const detail = err.details?.error;
			return typeof detail === 'string' ? detail : err.message;
		}
		return null;
	}

	async function savePrompt(): Promise<void> {
		if (saving || !dirty) return;
		saving = true;
		// Saving the unchanged default text keeps following the default.
		const toSave = draft.trim() === defaultPrompt.trim() ? '' : draft;
		try {
			await appSettingsStore.setInboxProcessingPrompt(toSave);
			templateError = null;
			draft = appSettingsStore.inboxProcessing.prompt || defaultPrompt;
			lastSeen = draft;
			toast.success($t('settings.inboxProcessing.toasts.promptSaved'));
		} catch (err) {
			const tplErr = templateErrorFrom(err);
			if (tplErr) {
				templateError = tplErr;
			} else {
				toast.error(describeError(err, $t('settings.inboxProcessing.toasts.promptFailed')));
			}
		} finally {
			saving = false;
		}
	}

	async function resetPrompt(): Promise<void> {
		if (saving) return;
		saving = true;
		try {
			await appSettingsStore.setInboxProcessingPrompt('');
			templateError = null;
			draft = defaultPrompt;
			lastSeen = draft;
			toast.success($t('settings.inboxProcessing.toasts.promptReset'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.inboxProcessing.toasts.promptFailed')));
		} finally {
			saving = false;
		}
	}

	async function preview(): Promise<void> {
		if (previewing) return;
		previewing = true;
		try {
			const res = await inboxProcessingApi.preview(getApiClient(), draft);
			templateError = null;
			previewText = res.rendered;
			previewOpen = true;
		} catch (err) {
			const tplErr = templateErrorFrom(err);
			if (tplErr) {
				templateError = tplErr;
			} else {
				toast.error(describeError(err, $t('settings.inboxProcessing.toasts.previewFailed')));
			}
		} finally {
			previewing = false;
		}
	}

	function formatDateTime(iso: string): string {
		try {
			return new Intl.DateTimeFormat($locale ?? undefined, {
				dateStyle: 'short',
				timeStyle: 'short',
				timeZone: configStore.value?.timezone ?? undefined
			}).format(new Date(iso));
		} catch {
			return iso;
		}
	}

	// Identifiers of the template language, not prose — shown verbatim.
	const variables = [
		{ name: '.Now', key: 'now' },
		{ name: '.Timezone', key: 'timezone' },
		{ name: '.Locale', key: 'locale' },
		{ name: '.Contexts', key: 'contexts' },
		{ name: '.Projects', key: 'projects' },
		{ name: '.Labels', key: 'labels' },
		{ name: '.Priorities', key: 'priorities' },
		{ name: '.Task', key: 'task' },
		{ name: 'join', key: 'join' }
	] as const;
	const templateExample = '{{range .Projects}}\n- id={{.ID}} "{{.Title}}" ({{.Context}}){{if .Labels}}: {{join .Labels ", "}}{{end}}\n{{end}}';
</script>

<div class="flex flex-col gap-4">
	<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
		<div class="flex items-start justify-between gap-3">
			<div class="flex flex-col gap-0.5">
				<h2 class="flex items-center gap-1.5 text-sm font-semibold">
					<SparkleIcon class="size-4 text-primary" weight="fill" />
					{$t('settings.inboxProcessing.heading')}
				</h2>
				<p class="text-xs text-muted-foreground">{$t('settings.inboxProcessing.description')}</p>
			</div>
			{#if status}
				<span
					class="shrink-0 rounded px-1.5 py-0.5 text-[11px] font-medium {!status.enabled
						? 'bg-muted text-muted-foreground'
						: paused
							? 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-300'
							: 'bg-green-500/15 text-green-700 dark:text-green-300'}"
					data-testid="inbox-processing-state"
				>
					{!status.enabled
						? $t('settings.inboxProcessing.status.disabled')
						: paused
							? $t('settings.inboxProcessing.status.paused')
							: $t('settings.inboxProcessing.status.enabled')}
				</span>
			{/if}
		</div>

		{#if statusError && !status}
			<p class="text-sm text-destructive">{$t('settings.inboxProcessing.status.loadFailed')}</p>
		{:else if status}
			{#if !status.enabled}
				<p class="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
					{$t('settings.inboxProcessing.status.disabledHint')}
				</p>
			{:else}
				<div class="flex items-start justify-between gap-3 rounded-md border border-border px-3 py-2">
					<div class="flex flex-col gap-0.5">
						<span class="text-xs font-medium">{$t('settings.inboxProcessing.pause.label')}</span>
						<span class="text-xs text-muted-foreground">{$t('settings.inboxProcessing.pause.hint')}</span>
					</div>
					<Switch
						checked={paused}
						disabled={pauseBusy}
						onCheckedChange={setPaused}
						aria-label={$t('settings.inboxProcessing.pause.label')}
					/>
				</div>
			{/if}
			<dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
				{#if status.model}
					<dt class="text-muted-foreground">{$t('settings.inboxProcessing.status.model')}</dt>
					<dd class="font-mono">{status.model}</dd>
				{/if}
				{#if status.apiHost}
					<dt class="text-muted-foreground">{$t('settings.inboxProcessing.status.provider')}</dt>
					<dd class="font-mono">{status.apiHost}</dd>
				{/if}
				<dt class="text-muted-foreground">{$t('settings.inboxProcessing.status.interval')}</dt>
				<dd class="font-mono">{status.interval}</dd>
				<dt class="text-muted-foreground">{$t('settings.inboxProcessing.status.batchLimit')}</dt>
				<dd>{status.batchLimit}</dd>
				<dt class="text-muted-foreground">{$t('settings.inboxProcessing.status.pending')}</dt>
				<dd data-testid="inbox-processing-pending">{status.pendingCount}</dd>
				{#if status.undecidedCount > 0}
					<dt class="text-muted-foreground">{$t('settings.inboxProcessing.status.undecided')}</dt>
					<dd data-testid="inbox-processing-undecided" title={$t('settings.inboxProcessing.status.undecidedHint')}>
						{status.undecidedCount}
					</dd>
				{/if}
				<dt class="text-muted-foreground">{$t('settings.inboxProcessing.status.lastRun')}</dt>
				<dd>
					{#if status.lastRunAt}
						{formatDateTime(status.lastRunAt)}{#if status.lastRunSummary}
							· {$t('settings.inboxProcessing.status.summary', { values: { ...status.lastRunSummary } })}{/if}
					{:else}
						{$t('settings.inboxProcessing.status.never')}
					{/if}
				</dd>
			</dl>
			{#if status.backoffUntil}
				<p class="text-xs text-yellow-700 dark:text-yellow-300">
					{$t('settings.inboxProcessing.status.backoffUntil', {
						values: { time: formatDateTime(status.backoffUntil) }
					})}
				</p>
			{/if}
			{#if status.lastError}
				<div class="flex flex-col gap-0.5 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2">
					<span class="text-[11px] font-medium text-destructive">{$t('settings.inboxProcessing.status.lastError')}</span>
					<span class="break-words font-mono text-xs text-destructive">{status.lastError}</span>
				</div>
			{/if}
			<div class="flex flex-wrap items-center gap-2">
				<Button
					size="sm"
					disabled={!canRun}
					title={status.enabled && status.pendingCount === 0 ? $t('settings.inboxProcessing.runDisabledHint') : undefined}
					onclick={runNow}
				>
					<PlayIcon class="size-3.5" weight="fill" />
					{running ? $t('settings.inboxProcessing.status.running') : $t('settings.inboxProcessing.run')}
				</Button>
			</div>
		{/if}
	</section>

	<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
		<div class="flex items-start justify-between gap-3">
			<div class="flex flex-col gap-0.5">
				<h2 class="text-sm font-semibold">{$t('settings.inboxProcessing.prompt.heading')}</h2>
				<p class="text-xs text-muted-foreground">{$t('settings.inboxProcessing.prompt.description')}</p>
			</div>
			<span class="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
				{usingDefault
					? $t('settings.inboxProcessing.prompt.usingDefault')
					: $t('settings.inboxProcessing.prompt.usingCustom')}
			</span>
		</div>

		<Textarea
			bind:value={draft}
			class="min-h-64 font-mono text-xs leading-relaxed md:text-xs"
			spellcheck={false}
			aria-label={$t('settings.inboxProcessing.prompt.heading')}
			aria-invalid={templateError ? true : undefined}
			oninput={() => (templateError = null)}
		/>
		{#if templateError}
			<p class="break-words font-mono text-xs text-destructive" data-testid="inbox-processing-template-error">
				{$t('settings.inboxProcessing.prompt.invalid', { values: { error: templateError } })}
			</p>
		{/if}

		<div class="flex flex-wrap items-center gap-2">
			<Button size="sm" disabled={!dirty || saving} onclick={savePrompt}>
				{$t('settings.inboxProcessing.prompt.save')}
			</Button>
			<Button size="sm" variant="outline" disabled={previewing || !draft.trim()} onclick={preview}>
				<EyeIcon class="size-3.5" />
				{$t('settings.inboxProcessing.prompt.preview')}
			</Button>
			<Button size="sm" variant="ghost" disabled={saving || (usingDefault && !dirty)} onclick={resetPrompt}>
				<ArrowCounterClockwiseIcon class="size-3.5" />
				{$t('settings.inboxProcessing.prompt.reset')}
			</Button>
			{#if dirty}
				<span class="text-xs text-muted-foreground">{$t('settings.inboxProcessing.prompt.unsaved')}</span>
			{/if}
		</div>

		<details class="group rounded-md border border-border px-3 py-2 text-xs">
			<summary class="cursor-pointer select-none font-medium">
				{$t('settings.inboxProcessing.variables.heading')}
			</summary>
			<dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
				{#each variables as v (v.name)}
					<dt class="font-mono">{v.name}</dt>
					<dd class="text-muted-foreground">{$t(`settings.inboxProcessing.variables.${v.key}`)}</dd>
				{/each}
			</dl>
			<p class="mt-3 font-medium">{$t('settings.inboxProcessing.variables.example')}</p>
			<pre class="mt-1 overflow-x-auto rounded bg-muted p-2 font-mono text-[11px]">{templateExample}</pre>
		</details>
	</section>

	<InboxProcessingLog refreshKey={logRefreshKey} />
</div>

<Sheet.Root bind:open={previewOpen}>
	<Sheet.Content side="right" class="w-full sm:max-w-2xl">
		<Sheet.Header>
			<Sheet.Title>{$t('settings.inboxProcessing.preview.title')}</Sheet.Title>
			<Sheet.Description>{$t('settings.inboxProcessing.preview.description')}</Sheet.Description>
		</Sheet.Header>
		<div class="min-h-0 flex-1 overflow-auto px-4">
			<pre class="whitespace-pre-wrap break-words rounded-md bg-muted p-3 font-mono text-xs" data-testid="inbox-processing-preview">{previewText}</pre>
		</div>
		<Sheet.Footer>
			<Sheet.Close>{$t('settings.inboxProcessing.preview.close')}</Sheet.Close>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
