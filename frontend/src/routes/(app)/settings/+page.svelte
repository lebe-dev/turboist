<script lang="ts">
	import { userPrefersMode, setMode } from 'mode-watcher';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import MonitorIcon from 'phosphor-svelte/lib/Monitor';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import SignOutIcon from 'phosphor-svelte/lib/SignOut';
	import QuestionIcon from 'phosphor-svelte/lib/Question';
	import CalendarBlankIcon from 'phosphor-svelte/lib/CalendarBlank';
	import ArrowsClockwiseIcon from 'phosphor-svelte/lib/ArrowsClockwise';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as HoverCard from '$lib/components/ui/hover-card';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
	import { calendars as calendarsApi } from '$lib/api/endpoints/calendars';
	import { getApiClient } from '$lib/api/client';
	import type { CalendarSettingsResponse } from '$lib/api/types';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { t, locale, SUPPORTED_LOCALES, localeLabel, type SupportedLocale } from '$lib/i18n';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	const appVersion = __APP_VERSION__;
	const auth = getAuthStore();

	let logoutAllBusy = $state(false);

	async function onLogoutAll(): Promise<void> {
		if (logoutAllBusy) return;
		logoutAllBusy = true;
		try {
			await auth.logoutAll();
			await goto(resolve('/login'));
		} catch {
			toast.error($t('settings.session.logoutAllFailed'));
		} finally {
			logoutAllBusy = false;
		}
	}

	type ThemeMode = 'light' | 'dark' | 'system';

	type ThemeOption = {
		value: ThemeMode;
		labelKey: string;
		descKey: string;
		icon: typeof SunIcon;
	};

	const themeOptions: ThemeOption[] = [
		{
			value: 'light',
			labelKey: 'settings.theme.light',
			descKey: 'settings.theme.lightDescription',
			icon: SunIcon
		},
		{
			value: 'dark',
			labelKey: 'settings.theme.dark',
			descKey: 'settings.theme.darkDescription',
			icon: MoonIcon
		},
		{
			value: 'system',
			labelKey: 'settings.theme.system',
			descKey: 'settings.theme.systemDescription',
			icon: MonitorIcon
		}
	];

	const currentTheme = $derived(userPrefersMode.current);

	const currentLocale = $derived(
		(settingsStore.locale || $locale || 'en') as SupportedLocale
	);

	let localeBusy = $state<SupportedLocale | null>(null);
	let calendarsState = $state<CalendarSettingsResponse | null>(null);
	let calendarsBusy = $state(false);
	let calendarsLoaded = $state(false);

	function toggleLabel(id: number) {
		const excluded = settingsStore.weeklyUnplannedExcludedLabelIds;
		const next = excluded.includes(id) ? excluded.filter((x) => x !== id) : [...excluded, id];
		settingsStore.setWeeklyUnplannedExcludedLabelIds(next).catch(console.error);
	}

	function toggleBugLabel(id: number) {
		const current = settingsStore.bugLabelIds;
		const next = current.includes(id) ? current.filter((x) => x !== id) : [...current, id];
		settingsStore.setBugLabelIds(next).catch(console.error);
	}

	async function setPublicView(v: boolean): Promise<void> {
		try {
			await settingsStore.setPublicView(v);
			toast.success($t('settings.privacy.updated'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.privacy.updateFailed');
			toast.error(message);
		}
	}

	async function loadCalendars(): Promise<void> {
		if (calendarsBusy) return;
		calendarsBusy = true;
		try {
			calendarsState = await calendarsApi.get(getApiClient());
			settingsStore.value = {
				...settingsStore.value,
				calendarEnabled: calendarsState.enabled
			};
			calendarsLoaded = true;
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.calendars.loadFailed');
			toast.error(message);
		} finally {
			calendarsBusy = false;
		}
	}

	async function setCalendarsEnabled(v: boolean): Promise<void> {
		calendarsBusy = true;
		try {
			calendarsState = await calendarsApi.setEnabled(getApiClient(), v);
			settingsStore.value = { ...settingsStore.value, calendarEnabled: calendarsState.enabled };
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.calendars.updateFailed');
			toast.error(message);
		} finally {
			calendarsBusy = false;
		}
	}

	async function connectGoogleCalendar(): Promise<void> {
		calendarsBusy = true;
		try {
			const res = await calendarsApi.googleStart(getApiClient());
			window.location.href = res.url;
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.calendars.connectFailed');
			toast.error(message);
			calendarsBusy = false;
		}
	}

	async function syncGoogleCalendar(): Promise<void> {
		calendarsBusy = true;
		try {
			calendarsState = await calendarsApi.googleSync(getApiClient());
			toast.success($t('settings.calendars.synced'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.calendars.syncFailed');
			toast.error(message);
		} finally {
			calendarsBusy = false;
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
			const message = err instanceof Error ? err.message : $t('settings.calendars.updateFailed');
			toast.error(message);
		}
	}

	async function disconnectCalendarAccount(id: number): Promise<void> {
		if (!calendarsState) return;
		calendarsBusy = true;
		try {
			await calendarsApi.deleteAccount(getApiClient(), id);
			calendarsState = await calendarsApi.get(getApiClient());
			toast.success($t('settings.calendars.disconnected'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.calendars.disconnectFailed');
			toast.error(message);
		} finally {
			calendarsBusy = false;
		}
	}

	let bannerDraft = $state(settingsStore.bannerText);

	const URL_RE = /^https?:\/\/\S+$/i;

	function onBannerPaste(e: ClipboardEvent): void {
		const url = e.clipboardData?.getData('text').trim() ?? '';
		if (!URL_RE.test(url)) return;
		const target = e.currentTarget as HTMLTextAreaElement;
		const start = target.selectionStart ?? bannerDraft.length;
		const end = target.selectionEnd ?? start;
		e.preventDefault();
		const label = bannerDraft.slice(start, end) || url;
		const insert = `[${label}](${url})`;
		bannerDraft = bannerDraft.slice(0, start) + insert + bannerDraft.slice(end);
		const cursor = start + insert.length;
		queueMicrotask(() => target.setSelectionRange(cursor, cursor));
	}

	async function saveBannerText(): Promise<void> {
		if (bannerDraft === settingsStore.bannerText) return;
		try {
			await settingsStore.setBannerText(bannerDraft);
			toast.success($t('settings.banner.toastSaved'));
		} catch (err) {
			bannerDraft = settingsStore.bannerText;
			const message = err instanceof Error ? err.message : $t('settings.banner.toastFailed');
			toast.error(message);
		}
	}

	async function setBannerPublished(v: boolean): Promise<void> {
		try {
			await settingsStore.setBannerPublished(v);
			toast.success($t('settings.banner.toastSaved'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.banner.toastFailed');
			toast.error(message);
		}
	}

	async function selectLocale(loc: SupportedLocale): Promise<void> {
		if (loc === currentLocale || localeBusy !== null) return;
		localeBusy = loc;
		try {
			await settingsStore.setLocale(loc);
			toast.success($t('settings.language.updated'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.language.updateFailed');
			toast.error(message);
		} finally {
			localeBusy = null;
		}
	}

	$effect(() => {
		if (!calendarsLoaded) void loadCalendars();
	});
</script>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8 sm:px-6">
	<header class="flex flex-col gap-1">
		<h1 class="text-xl font-semibold tracking-tight">{$t('settings.title')}</h1>
		<p class="text-sm text-muted-foreground">{$t('settings.subtitle')}</p>
	</header>

	<Tabs.Root value="general" class="flex flex-col gap-4">
		<Tabs.List variant="line">
			<Tabs.Trigger value="general">{$t('settings.tabs.general')}</Tabs.Trigger>
			<Tabs.Trigger value="labels">{$t('settings.tabs.labels')}</Tabs.Trigger>
			<Tabs.Trigger value="calendars">{$t('settings.tabs.calendars')}</Tabs.Trigger>
			<Tabs.Trigger value="project">{$t('settings.tabs.project')}</Tabs.Trigger>
			<Tabs.Trigger value="privacy">{$t('settings.tabs.privacy')}</Tabs.Trigger>
			<Tabs.Trigger value="session">{$t('settings.tabs.session')}</Tabs.Trigger>
		</Tabs.List>

		<Tabs.Content value="general" class="flex flex-col gap-4">
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.theme.heading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.theme.description')}</p>
				</div>
				<div class="grid gap-2 sm:grid-cols-3" role="radiogroup" aria-label={$t('settings.theme.ariaLabel')}>
					{#each themeOptions as option (option.value)}
						{@const Icon = option.icon}
						{@const active = currentTheme === option.value}
						<button
							type="button"
							role="radio"
							aria-checked={active}
							onclick={() => setMode(option.value)}
							class="flex flex-col items-start gap-2 rounded-md border p-3 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 {active ? 'border-foreground/30 bg-muted' : 'border-border'}"
						>
							<span class="flex items-center gap-2">
								<Icon class="size-4" weight={active ? 'fill' : 'regular'} />
								<span class="text-sm font-medium">{$t(option.labelKey)}</span>
							</span>
							<span class="text-xs text-muted-foreground">{$t(option.descKey)}</span>
						</button>
					{/each}
				</div>
			</section>

			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.language.heading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.language.description')}</p>
				</div>
				<div
					class="grid gap-2 sm:grid-cols-2"
					role="radiogroup"
					aria-label={$t('settings.language.ariaLabel')}
				>
					{#each SUPPORTED_LOCALES as loc (loc)}
						{@const active = currentLocale === loc}
						<button
							type="button"
							role="radio"
							aria-checked={active}
							onclick={() => selectLocale(loc)}
							disabled={localeBusy !== null}
							class="flex items-center justify-between gap-2 rounded-md border p-3 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-60 {active ? 'border-foreground/30 bg-muted' : 'border-border'}"
						>
							<span class="text-sm font-medium">{localeLabel(loc)}</span>
							{#if active}
								<CheckIcon class="size-4 text-foreground/50" weight="bold" />
							{/if}
						</button>
					{/each}
				</div>
			</section>
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex items-start justify-between gap-3">
					<div class="flex flex-col gap-0.5">
						<h2 class="text-sm font-semibold">{$t('settings.banner.heading')}</h2>
						<p class="text-xs text-muted-foreground">{$t('settings.banner.description')}</p>
					</div>
					<Switch
						checked={settingsStore.bannerPublished}
						onCheckedChange={setBannerPublished}
						aria-label={$t('settings.banner.publishLabel')}
					/>
				</div>
				<label class="flex flex-col gap-1.5">
					<span class="text-xs font-medium text-muted-foreground">{$t('settings.banner.textLabel')}</span>
					<textarea
						bind:value={bannerDraft}
						onblur={saveBannerText}
						onpaste={onBannerPaste}
						placeholder={$t('settings.banner.textPlaceholder')}
						rows="3"
						class="resize-y rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
					></textarea>
				</label>
			</section>

			<section class="flex items-center justify-between rounded-lg border border-border bg-card px-5 py-4 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.version.heading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.version.description')}</p>
				</div>
				<span class="font-mono text-sm text-muted-foreground">v{appVersion}</span>
			</section>
		</Tabs.Content>

		<Tabs.Content value="calendars" class="flex flex-col gap-4">
			<section class="flex flex-col gap-4 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex items-start justify-between gap-3">
					<div class="flex flex-col gap-0.5">
						<h2 class="text-sm font-semibold">{$t('settings.calendars.heading')}</h2>
						<p class="text-xs text-muted-foreground">{$t('settings.calendars.description')}</p>
					</div>
					<Switch
						checked={settingsStore.calendarEnabled}
						disabled={calendarsBusy}
						onCheckedChange={setCalendarsEnabled}
						aria-label={$t('settings.calendars.enableLabel')}
					/>
				</div>

				{#if calendarsState && !calendarsState.googleConfigured}
					<p class="rounded-md border border-border/60 bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
						{$t('settings.calendars.googleNotConfigured')}
					</p>
				{/if}

				<div class="flex flex-wrap gap-2">
					<button
						type="button"
						onclick={connectGoogleCalendar}
						disabled={calendarsBusy || calendarsState?.googleConfigured === false}
						class="inline-flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm transition-colors hover:border-foreground/30 hover:bg-muted/50 disabled:cursor-not-allowed disabled:opacity-50"
					>
						<CalendarBlankIcon class="size-4" />
						{$t('settings.calendars.connectGoogle')}
					</button>
					<button
						type="button"
						onclick={syncGoogleCalendar}
						disabled={calendarsBusy || (calendarsState?.accounts.length ?? 0) === 0}
						class="inline-flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm transition-colors hover:border-foreground/30 hover:bg-muted/50 disabled:cursor-not-allowed disabled:opacity-50"
					>
						<ArrowsClockwiseIcon class="size-4" />
						{$t('settings.calendars.sync')}
					</button>
				</div>
			</section>

			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.calendars.sourcesHeading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.calendars.sourcesDescription')}</p>
				</div>
				{#if calendarsBusy && !calendarsState}
					<p class="text-sm text-muted-foreground">{$t('common.loading')}</p>
				{:else if !calendarsState || calendarsState.sources.length === 0}
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
									<span class="h-2.5 w-2.5 shrink-0 rounded-full" style={`background:${source.color || '#9ca3af'}`}></span>
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

			{#if calendarsState && calendarsState.accounts.length > 0}
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
									disabled={calendarsBusy}
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
		</Tabs.Content>

		<Tabs.Content value="labels">
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.weekly.heading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.weekly.description')}</p>
				</div>
				{#if labelsStore.items.length === 0}
					<p class="text-sm text-muted-foreground">{$t('settings.weekly.empty')}</p>
				{:else}
					<div class="flex flex-col gap-1">
						{#each labelsStore.items as label (label.id)}
							{@const excluded = settingsStore.weeklyUnplannedExcludedLabelIds.includes(label.id)}
							<button
								type="button"
								onclick={() => toggleLabel(label.id)}
								class="flex items-center justify-between rounded-md px-3 py-2 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
								class:bg-muted={excluded}
								aria-pressed={excluded}
							>
								<span
									class="inline-flex items-center rounded-full bg-accent/50 px-2 py-0.5 text-[11px] font-medium text-muted-foreground"
								>
									{label.name}
								</span>
								{#if excluded}
									<CheckIcon class="size-4 text-foreground/50" weight="bold" />
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			</section>
		</Tabs.Content>

		<Tabs.Content value="project">
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.project.bugLabelsHeading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.project.bugLabelsDescription')}</p>
				</div>
				{#if labelsStore.items.length === 0}
					<p class="text-sm text-muted-foreground">{$t('settings.project.bugLabelsEmpty')}</p>
				{:else}
					<div class="flex flex-col gap-1">
						{#each labelsStore.items as label (label.id)}
							{@const active = settingsStore.bugLabelIds.includes(label.id)}
							<button
								type="button"
								onclick={() => toggleBugLabel(label.id)}
								class="flex items-center justify-between rounded-md px-3 py-2 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
								class:bg-muted={active}
								aria-pressed={active}
							>
								<span
									class="inline-flex items-center rounded-full bg-accent/50 px-2 py-0.5 text-[11px] font-medium text-muted-foreground"
								>
									{label.name}
								</span>
								{#if active}
									<CheckIcon class="size-4 text-foreground/50" weight="bold" />
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			</section>
		</Tabs.Content>

		<Tabs.Content value="privacy" class="flex flex-col gap-4">
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex items-start justify-between gap-3">
					<div class="flex flex-col gap-0.5">
						<div class="flex items-center gap-1.5">
							<h2 class="text-sm font-semibold">{$t('settings.privacy.heading')}</h2>
							<HoverCard.Root>
								<HoverCard.Trigger>
									<QuestionIcon
										class="size-4 cursor-help text-muted-foreground transition-colors hover:text-foreground"
										aria-label={$t('settings.privacy.hintAria')}
									/>
								</HoverCard.Trigger>
								<HoverCard.Content class="w-80 text-xs leading-relaxed">
									{$t('settings.privacy.hint')}
								</HoverCard.Content>
							</HoverCard.Root>
						</div>
						<p class="text-xs text-muted-foreground">{$t('settings.privacy.description')}</p>
					</div>
					<Switch
						checked={settingsStore.publicView}
						onCheckedChange={setPublicView}
						aria-label={$t('settings.privacy.toggle')}
					/>
				</div>
			</section>
		</Tabs.Content>

		<Tabs.Content value="session" class="flex flex-col gap-4">
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.session.heading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.session.description')}</p>
				</div>
				<div>
					<button
						type="button"
						onclick={onLogoutAll}
						disabled={logoutAllBusy}
						class="flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-muted-foreground transition-colors hover:border-foreground/30 hover:bg-muted/50 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
					>
						<SignOutIcon class="size-4 shrink-0" />
						{logoutAllBusy ? $t('settings.session.loggingOut') : $t('settings.session.logoutAll')}
					</button>
				</div>
			</section>
		</Tabs.Content>
	</Tabs.Root>
</div>
