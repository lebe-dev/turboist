<script lang="ts">
	import { onMount } from 'svelte';
	import { toast } from 'svelte-sonner';
	import CalendarBlankIcon from 'phosphor-svelte/lib/CalendarBlank';
	import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwise';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import QuestionIcon from 'phosphor-svelte/lib/Question';
	import { t } from '$lib/i18n';
	import { calendars as calendarsApi } from '$lib/api/endpoints/calendars';
	import { getApiClient } from '$lib/api/client';
	import type { CalendarSettingsResponse } from '$lib/api/types';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Switch } from '$lib/components/ui/switch';
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import * as HoverCard from '$lib/components/ui/hover-card';

	let calendarsState = $state<CalendarSettingsResponse | null>(null);
	let loading = $state(true);
	let busy = $state(false);
	let googleClientIdDraft = $state('');
	let googleClientSecretDraft = $state('');
	let deleteConfigOpen = $state(false);
	let redirectUri = $state('');

	const googleCredentialsUrl = 'https://console.cloud.google.com/apis/credentials';

	onMount(async () => {
		redirectUri = `${window.location.origin}/api/v1/calendars/google/callback`;
		await load();
	});

	async function load(): Promise<void> {
		loading = true;
		try {
			await refreshState();
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.loadFailed')));
		} finally {
			loading = false;
		}
	}

	async function refreshState(): Promise<void> {
		calendarsState = await calendarsApi.get(getApiClient());
		googleClientIdDraft = '';
		googleClientSecretDraft = '';
		settingsStore.value = {
			...settingsStore.value,
			calendarEnabled: calendarsState.enabled
		};
	}

	async function setCalendarsEnabled(enabled: boolean): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			calendarsState = await calendarsApi.setEnabled(getApiClient(), enabled);
			settingsStore.value = { ...settingsStore.value, calendarEnabled: calendarsState.enabled };
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.updateFailed')));
		} finally {
			busy = false;
		}
	}

	async function setHidePastEvents(enabled: boolean): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			await settingsStore.setCalendarHidePastEvents(enabled);
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.updateFailed')));
		} finally {
			busy = false;
		}
	}

	async function saveGoogleCalendarConfig(): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			calendarsState = await calendarsApi.saveGoogleConfig(
				getApiClient(),
				googleClientIdDraft,
				googleClientSecretDraft
			);
			googleClientIdDraft = '';
			googleClientSecretDraft = '';
			toast.success($t('settings.calendars.configSaved'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.configSaveFailed')));
		} finally {
			busy = false;
		}
	}

	async function deleteGoogleCalendarConfig(): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			calendarsState = await calendarsApi.deleteGoogleConfig(getApiClient());
			googleClientIdDraft = '';
			googleClientSecretDraft = '';
			toast.success($t('settings.calendars.configDeleted'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.configDeleteFailed')));
		} finally {
			busy = false;
			deleteConfigOpen = false;
		}
	}

	async function connectGoogleCalendar(): Promise<void> {
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

	async function syncGoogleCalendar(): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			calendarsState = await calendarsApi.googleSync(getApiClient());
			toast.success($t('settings.calendars.synced'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.syncFailed')));
		} finally {
			busy = false;
		}
	}

	async function toggleCalendarSource(id: number, selected: boolean): Promise<void> {
		if (!calendarsState) return;
		const previous = calendarsState;
		calendarsState = {
			...calendarsState,
			sources: calendarsState.sources.map((s) => (s.id === id ? { ...s, selected } : s))
		};
		try {
			const updated = await calendarsApi.setSourceSelected(getApiClient(), id, selected);
			calendarsState = {
				...calendarsState,
				sources: calendarsState.sources.map((s) => (s.id === id ? updated : s))
			};
		} catch (err) {
			calendarsState = previous;
			toast.error(describeError(err, $t('settings.calendars.updateFailed')));
		}
	}

	async function disconnectCalendarAccount(id: number): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			await calendarsApi.deleteAccount(getApiClient(), id);
			await refreshState();
			toast.success($t('settings.calendars.disconnected'));
		} catch (err) {
			toast.error(describeError(err, $t('settings.calendars.disconnectFailed')));
		} finally {
			busy = false;
		}
	}

	const canSaveConfig = $derived(
		!busy &&
			!!calendarsState &&
			!calendarsState.googleConfigFromEnv &&
			(calendarsState.googleClientIdConfigured || googleClientIdDraft.trim() !== '') &&
			(calendarsState.googleClientSecretConfigured || googleClientSecretDraft.trim() !== '')
	);
</script>

<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.calendars.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.calendars.description')}</p>
		</div>
		<Switch
			checked={settingsStore.calendarEnabled}
			disabled={busy || loading}
			onCheckedChange={setCalendarsEnabled}
			aria-label={$t('settings.calendars.enableLabel')}
		/>
	</div>
</section>

{#if loading}
	<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
		<div class="text-xs text-muted-foreground">…</div>
	</section>
{:else if calendarsState?.enabled}
	<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
		<div class="flex items-start justify-between gap-3">
			<div class="flex flex-col gap-0.5">
				<h3 class="text-sm font-medium">{$t('settings.calendars.hidePastEvents')}</h3>
				<p class="text-xs text-muted-foreground">{$t('settings.calendars.hidePastEventsDescription')}</p>
			</div>
			<Switch
				checked={settingsStore.calendarHidePastEvents}
				disabled={busy || loading}
				onCheckedChange={setHidePastEvents}
				aria-label={$t('settings.calendars.hidePastEvents')}
			/>
		</div>

		{#if !calendarsState.googleConfigured}
			<p class="rounded-md border border-border/60 bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
				{$t('settings.calendars.googleNotConfigured')}
			</p>
		{/if}

		<form
			class="grid gap-3 border-t border-border/60 pt-4"
			onsubmit={(e) => {
				e.preventDefault();
				void saveGoogleCalendarConfig();
			}}
		>
			<div class="flex flex-col gap-1">
				<div class="flex items-center gap-1.5">
					<h3 class="text-sm font-medium">{$t('settings.calendars.configHeading')}</h3>
					<HoverCard.Root>
						<HoverCard.Trigger>
							<QuestionIcon
								class="size-4 cursor-help text-muted-foreground transition-colors hover:text-foreground"
								aria-label={$t('settings.calendars.configHelpAria')}
							/>
						</HoverCard.Trigger>
						<HoverCard.Content class="w-80 text-xs leading-relaxed">
							<div class="flex flex-col gap-2">
								<p class="font-medium text-foreground">{$t('settings.calendars.configHelpTitle')}</p>
								<ol class="ml-4 list-decimal space-y-1 text-muted-foreground">
									<li>{$t('settings.calendars.configHelpEnableApi')}</li>
									<li>{$t('settings.calendars.configHelpCreateClient')}</li>
									<li>{$t('settings.calendars.configHelpCopyKeys')}</li>
								</ol>
								{#if redirectUri}
									<div class="rounded border border-border/60 bg-muted/40 px-2 py-1.5">
										<p class="mb-1 text-[11px] font-medium text-muted-foreground">
											{$t('settings.calendars.configHelpRedirectLabel')}
										</p>
										<code class="break-all font-mono text-[11px] text-foreground">{redirectUri}</code>
									</div>
								{/if}
								<a
									href={googleCredentialsUrl}
									target="_blank"
									rel="noreferrer"
									class="font-medium text-foreground underline underline-offset-2"
								>
									{$t('settings.calendars.configHelpLink')}
								</a>
							</div>
						</HoverCard.Content>
					</HoverCard.Root>
				</div>
				<p class="text-xs text-muted-foreground">
					{calendarsState.googleConfigFromEnv
						? $t('settings.calendars.configFromEnv')
						: $t('settings.calendars.configDescription')}
				</p>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<label class="flex flex-col gap-1.5">
					<span class="text-xs font-medium text-muted-foreground">{$t('settings.calendars.clientId')}</span>
					<Input
						bind:value={googleClientIdDraft}
						disabled={busy || calendarsState.googleConfigFromEnv}
						placeholder={calendarsState.googleClientIdConfigured
							? $t('settings.calendars.clientIdSaved')
							: ''}
						autocomplete="off"
						autocapitalize="none"
						autocorrect="off"
						spellcheck={false}
					/>
				</label>
				<label class="flex flex-col gap-1.5">
					<span class="text-xs font-medium text-muted-foreground">{$t('settings.calendars.clientSecret')}</span>
					<Input
						type="text"
						bind:value={googleClientSecretDraft}
						disabled={busy || calendarsState.googleConfigFromEnv}
						placeholder={calendarsState.googleClientSecretConfigured
							? $t('settings.calendars.secretSaved')
							: ''}
						autocomplete="off"
						autocapitalize="none"
						autocorrect="off"
						spellcheck={false}
					/>
				</label>
			</div>
			{#if !calendarsState.googleConfigFromEnv}
				<div class="flex flex-wrap gap-2">
					<Button type="submit" variant="outline" disabled={!canSaveConfig}>
						<CheckIcon class="size-4" />
						{$t('settings.calendars.saveConfig')}
					</Button>
					{#if calendarsState.googleClientIdConfigured || calendarsState.googleClientSecretConfigured}
						<Button
							type="button"
							variant="outline"
							disabled={busy}
							class="border-destructive/35 text-destructive hover:bg-destructive/10"
							onclick={() => (deleteConfigOpen = true)}
						>
							<TrashIcon class="size-4" />
							{$t('settings.calendars.deleteConfig')}
						</Button>
					{/if}
				</div>
			{/if}
		</form>

		<div class="flex flex-wrap gap-2">
			<Button
				type="button"
				variant="outline"
				onclick={connectGoogleCalendar}
				disabled={busy || calendarsState.googleConfigured === false}
			>
				<CalendarBlankIcon class="size-4" />
				{$t('settings.calendars.connectGoogle')}
			</Button>
			<Button
				type="button"
				variant="outline"
				onclick={syncGoogleCalendar}
				disabled={busy || calendarsState.accounts.length === 0}
			>
				<ArrowsClockwiseIcon class="size-4" />
				{$t('settings.calendars.sync')}
			</Button>
		</div>
	</section>

	<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.calendars.sourcesHeading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.calendars.sourcesDescription')}</p>
		</div>
		{#if calendarsState.sources.length === 0}
			<p class="text-sm text-muted-foreground">{$t('settings.calendars.empty')}</p>
		{:else}
			<div class="flex flex-col gap-1">
				{#each calendarsState.sources as source (source.id)}
					<button
						type="button"
						onclick={() => toggleCalendarSource(source.id, !source.selected)}
						class="flex items-center justify-between gap-3 rounded-md px-3 py-2 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
						class:bg-muted={source.selected}
						aria-pressed={source.selected}
					>
						<span class="flex min-w-0 items-center gap-2">
							<span
								class="h-2.5 w-2.5 shrink-0 rounded-full"
								style={`background:${source.color || '#9ca3af'}`}
							></span>
							<span class="min-w-0 truncate text-sm">{source.summary}</span>
							{#if source.isPrimary}
								<span class="shrink-0 rounded bg-accent px-1.5 py-0.5 text-[10px] text-muted-foreground">
									{$t('settings.calendars.primary')}
								</span>
							{/if}
						</span>
						{#if source.selected}
							<CheckIcon class="size-4 shrink-0 text-foreground/50" weight="bold" />
						{/if}
					</button>
				{/each}
			</div>
		{/if}
	</section>

	{#if calendarsState.accounts.length > 0}
		<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
			<div class="flex flex-col gap-0.5">
				<h2 class="text-sm font-semibold">{$t('settings.calendars.accountsHeading')}</h2>
				<p class="text-xs text-muted-foreground">{$t('settings.calendars.accountsDescription')}</p>
			</div>
			<div class="flex flex-col gap-1">
				{#each calendarsState.accounts as account (account.id)}
					<div class="flex items-center justify-between gap-3 rounded-md px-3 py-2">
						<div class="min-w-0">
							<p class="truncate text-sm font-medium">{account.displayName || account.email || 'Google Calendar'}</p>
							{#if account.email}
								<p class="truncate text-xs text-muted-foreground">{account.email}</p>
							{/if}
						</div>
						<button
							type="button"
							onclick={() => disconnectCalendarAccount(account.id)}
							disabled={busy}
							aria-label={$t('settings.calendars.disconnect')}
							title={$t('settings.calendars.disconnect')}
							class="inline-flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
						>
							<TrashIcon class="size-4" />
						</button>
					</div>
				{/each}
			</div>
		</section>
	{/if}
{/if}

<AlertDialog.Root bind:open={deleteConfigOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('settings.calendars.confirmDeleteConfigTitle')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('settings.calendars.confirmDeleteConfigDesc')}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel>{$t('common.cancel')}</AlertDialog.Cancel>
			<AlertDialog.Action onclick={deleteGoogleCalendarConfig}>
				{$t('settings.calendars.deleteConfig')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
