<script lang="ts">
	import { userPrefersMode, setMode } from 'mode-watcher';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import MonitorIcon from 'phosphor-svelte/lib/Monitor';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import QuestionIcon from 'phosphor-svelte/lib/Question';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Select from '$lib/components/ui/select';
	import * as HoverCard from '$lib/components/ui/hover-card';
	import ApiTokensSection from '$lib/components/settings/ApiTokensSection.svelte';
	import BackupRestoreSection from '$lib/components/settings/BackupRestoreSection.svelte';
	import GoogleCalendarSection from '$lib/components/settings/GoogleCalendarSection.svelte';
	import LogsSection from '$lib/components/settings/LogsSection.svelte';
	import ProjectSuggestionsSection from '$lib/components/settings/ProjectSuggestionsSection.svelte';
	import SessionsSection from '$lib/components/settings/SessionsSection.svelte';
	import TemplatesSection from '$lib/components/settings/TemplatesSection.svelte';
	import TwoFactorSection from '$lib/components/settings/TwoFactorSection.svelte';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { calendarReauthStore } from '$lib/stores/calendarReauth.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { isLabelVisible } from '$lib/utils/visibility';
	import { appSettingsStore } from '$lib/stores/appSettings.svelte';
	import type { AutoLabelRule, BannerDayPart } from '$lib/api/types';
	import { BANNER_DAY_PART_OPTIONS } from '$lib/utils/banner';
	import { iconFor as dayPartIcon } from '$lib/components/view/dayPartIcon';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import LabelPicker from '$lib/components/label/LabelPicker.svelte';
	import { t, locale, SUPPORTED_LOCALES, localeLabel, type SupportedLocale } from '$lib/i18n';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';

	const appVersion = __APP_VERSION__;
	const totpAvailable = $derived(configStore.value?.totpAvailable ?? false);

	const settingsTabs = ['general', 'labels', 'templates', 'calendars', 'project', 'troiki', 'privacy', 'security', 'api', 'backup', 'logs'] as const;
	type SettingsTab = (typeof settingsTabs)[number];

	let activeTab = $state<SettingsTab>('general');

	function isSettingsTab(value: string | null): value is SettingsTab {
		return !!value && (settingsTabs as readonly string[]).includes(value);
	}

	$effect(() => {
		const tab = page.url.searchParams.get('tab');
		if (isSettingsTab(tab)) {
			activeTab = tab;
			return;
		}
		if (page.url.searchParams.has('calendar')) {
			activeTab = 'calendars';
			// Returning from a successful (re)connect means the OAuth grant is
			// healthy again — dismiss any pending re-authorization notice.
			if (page.url.searchParams.get('calendar') === 'connected') {
				calendarReauthStore.clear();
			}
		}
	});

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

	function onWeeklyExcludedLabelsChange(next: number[]): void {
		settingsStore.setWeeklyUnplannedExcludedLabelIds(next).catch(console.error);
	}

	function onBugLabelsChange(next: number[]): void {
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

	async function setTroikiEnabled(v: boolean): Promise<void> {
		try {
			await settingsStore.setTroikiEnabled(v);
			toast.success($t('settings.troiki.updated'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.troiki.updateFailed');
			toast.error(message);
		}
	}

	const tabItems = $derived([
		{ value: 'general', labelKey: 'settings.tabs.general' },
		{ value: 'labels', labelKey: 'settings.tabs.labels' },
		{ value: 'templates', labelKey: 'settings.tabs.templates' },
		{ value: 'calendars', labelKey: 'settings.tabs.calendars' },
		{ value: 'project', labelKey: 'settings.tabs.project' },
		{ value: 'troiki', labelKey: 'settings.tabs.troiki' },
		{ value: 'privacy', labelKey: 'settings.tabs.privacy' },
		{ value: 'security', labelKey: 'settings.tabs.security' },
		{ value: 'api', labelKey: 'settings.tabs.api' },
		{ value: 'backup', labelKey: 'settings.tabs.backup' },
		{ value: 'logs', labelKey: 'settings.tabs.logs' }
	]);

	const activeTabLabel = $derived(
		$t(tabItems.find((t) => t.value === activeTab)?.labelKey ?? 'settings.tabs.general')
	);

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

	const bannerDayPart = $derived(settingsStore.bannerDayPart);

	async function setBannerDayPart(part: BannerDayPart): Promise<void> {
		if (part === settingsStore.bannerDayPart) return;
		try {
			await settingsStore.setBannerDayPart(part);
			toast.success($t('settings.banner.toastSaved'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.banner.toastFailed');
			toast.error(message);
		}
	}

	let autoLabelsDraft = $state<AutoLabelRule[]>(
		appSettingsStore.autoLabels.map((r) => ({ ...r, labelIds: [...r.labelIds] }))
	);
	let autoLabelsBusy = $state(false);

	const allLabels = $derived(
		[...labelsStore.favourites, ...labelsStore.rest].filter((l) =>
			isLabelVisible(l, settingsStore.publicView)
		)
	);

	const autoLabelsDirty = $derived.by(() => {
		const a = autoLabelsDraft;
		const b = appSettingsStore.autoLabels;
		if (a.length !== b.length) return true;
		for (let i = 0; i < a.length; i++) {
			if (a[i].mask !== b[i].mask || a[i].ignoreCase !== b[i].ignoreCase) return true;
			if (a[i].labelIds.length !== b[i].labelIds.length) return true;
			for (let j = 0; j < a[i].labelIds.length; j++) {
				if (a[i].labelIds[j] !== b[i].labelIds[j]) return true;
			}
		}
		return false;
	});

	// Set when a new rule is appended; consumed by the mask input's attach to autofocus it
	let pendingRuleFocus = $state(false);

	function addAutoLabelRule(): void {
		autoLabelsDraft = [...autoLabelsDraft, { mask: '', labelIds: [], ignoreCase: true }];
		pendingRuleFocus = true;
	}

	function focusNewMask(el: HTMLInputElement): void {
		// Only the visible layout (mobile vs desktop) has a non-null offsetParent
		if (pendingRuleFocus && el.offsetParent) {
			pendingRuleFocus = false;
			el.focus();
		}
	}

	function removeAutoLabelRule(idx: number): void {
		autoLabelsDraft = autoLabelsDraft.filter((_, i) => i !== idx);
	}

	async function saveAutoLabels(): Promise<void> {
		const cleaned = autoLabelsDraft.map((r) => ({
			mask: r.mask.trim(),
			labelIds: r.labelIds,
			ignoreCase: r.ignoreCase
		}));
		if (cleaned.some((r) => r.mask === '' || r.labelIds.length === 0)) {
			toast.error($t('settings.autoLabels.toastEmptyFields'));
			return;
		}
		autoLabelsBusy = true;
		try {
			await appSettingsStore.setAutoLabels(cleaned);
			toast.success($t('settings.autoLabels.toastSaved'));
		} catch (err) {
			const message = err instanceof Error ? err.message : $t('settings.autoLabels.toastFailed');
			toast.error(message);
		} finally {
			autoLabelsBusy = false;
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

</script>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8 sm:px-6">
	<header class="flex flex-col gap-1">
		<h1 class="text-xl font-semibold tracking-tight">{$t('settings.title')}</h1>
		<p class="text-sm text-muted-foreground">{$t('settings.subtitle')}</p>
	</header>

	<Tabs.Root bind:value={activeTab} class="flex flex-col gap-4">
		<Select.Root type="single" bind:value={activeTab}>
			<Select.Trigger class="!h-9 w-full text-sm font-medium" aria-label={$t('settings.title')}>{activeTabLabel}</Select.Trigger>
			<Select.Content>
				{#each tabItems as item (item.value)}
					<Select.Item class="py-3 text-sm" value={item.value} label={$t(item.labelKey)}>{$t(item.labelKey)}</Select.Item>
				{/each}
			</Select.Content>
		</Select.Root>

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
				<div class="flex flex-col gap-1.5">
					<span class="text-xs font-medium text-muted-foreground">{$t('settings.banner.dayPart.label')}</span>
					<div
						class="grid gap-2 sm:grid-cols-2 lg:grid-cols-4"
						role="radiogroup"
						aria-label={$t('settings.banner.dayPart.label')}
					>
						{#each BANNER_DAY_PART_OPTIONS as option (option.value)}
							{@const Icon = dayPartIcon(option.value === '' ? 'none' : option.value)}
							{@const active = bannerDayPart === option.value}
							<button
								type="button"
								role="radio"
								aria-checked={active}
								onclick={() => setBannerDayPart(option.value)}
								class="flex items-center gap-2 rounded-md border p-3 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 {active ? 'border-foreground/30 bg-muted' : 'border-border'}"
							>
								<Icon class="size-4 shrink-0" weight={active ? 'fill' : 'regular'} />
								<span class="truncate text-sm font-medium">{$t(option.labelKey)}</span>
							</button>
						{/each}
					</div>
					<p class="text-xs text-muted-foreground">{$t('settings.banner.dayPart.hint')}</p>
				</div>
			</section>

			<section class="flex flex-col gap-2 rounded-lg border border-border bg-card px-5 py-4 shadow-sm">
				<a
					href={resolve('/terms-of-service')}
					class="text-sm text-muted-foreground hover:text-foreground hover:underline"
				>
					{$t('legal.tos.title')}
				</a>
				<a
					href={resolve('/privacy-policy')}
					class="text-sm text-muted-foreground hover:text-foreground hover:underline"
				>
					{$t('legal.privacy.title')}
				</a>
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
			<GoogleCalendarSection />
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
					<LabelPicker
						value={settingsStore.weeklyUnplannedExcludedLabelIds}
						onValueChange={onWeeklyExcludedLabelsChange}
					/>
				{/if}
			</section>

			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.autoLabels.heading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.autoLabels.description')}</p>
				</div>

				{#if autoLabelsDraft.length === 0}
					<p class="text-sm text-muted-foreground">{$t('settings.autoLabels.empty')}</p>
				{:else}
					<div class="flex flex-col gap-2">
						<div class="hidden sm:grid grid-cols-[1fr_1fr_auto_auto] items-center gap-2 px-1 text-[11px] font-medium text-muted-foreground">
							<span>{$t('settings.autoLabels.mask')}</span>
							<span>{$t('settings.autoLabels.labels')}</span>
							<span>{$t('settings.autoLabels.ignoreCase')}</span>
							<span class="sr-only">{$t('settings.autoLabels.remove')}</span>
						</div>
						{#each autoLabelsDraft as rule, idx (idx)}
							<!-- mobile card -->
							<div class="flex flex-col gap-2 rounded-md border border-border p-3 sm:hidden">
								<div class="flex items-center gap-2">
									<input
										type="text"
										bind:value={rule.mask}
										{@attach (el) => {
											if (idx === autoLabelsDraft.length - 1) focusNewMask(el);
										}}
										placeholder={$t('settings.autoLabels.maskPlaceholder')}
										class="min-w-0 flex-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
									/>
									<Switch
										checked={rule.ignoreCase}
										onCheckedChange={(v) => (rule.ignoreCase = v)}
										aria-label={$t('settings.autoLabels.ignoreCase')}
									/>
									<button
										type="button"
										onclick={() => removeAutoLabelRule(idx)}
										aria-label={$t('settings.autoLabels.remove')}
										class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-destructive focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
									>
										<TrashIcon class="size-4" />
									</button>
								</div>
								{#if allLabels.length > 0}
									<LabelPicker bind:value={rule.labelIds} />
								{:else}
									<span class="text-xs text-muted-foreground">
										{$t('settings.autoLabels.noLabelsAvailable')}
									</span>
								{/if}
							</div>
							<!-- desktop row (unchanged) -->
							<div class="hidden sm:grid grid-cols-[1fr_1fr_auto_auto] items-center gap-2">
								<input
									type="text"
									bind:value={rule.mask}
									{@attach (el) => {
										if (idx === autoLabelsDraft.length - 1) focusNewMask(el);
									}}
									placeholder={$t('settings.autoLabels.maskPlaceholder')}
									class="rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
								/>
								{#if allLabels.length > 0}
									<LabelPicker bind:value={rule.labelIds} />
								{:else}
									<span class="text-xs text-muted-foreground">
										{$t('settings.autoLabels.noLabelsAvailable')}
									</span>
								{/if}
								<Switch
									checked={rule.ignoreCase}
									onCheckedChange={(v) => (rule.ignoreCase = v)}
									aria-label={$t('settings.autoLabels.ignoreCase')}
								/>
								<button
									type="button"
									onclick={() => removeAutoLabelRule(idx)}
									aria-label={$t('settings.autoLabels.remove')}
									class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-destructive focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
								>
									<TrashIcon class="size-4" />
								</button>
							</div>
						{/each}
					</div>
				{/if}

				<div class="flex items-center justify-between gap-2 pt-1">
					<button
						type="button"
						onclick={addAutoLabelRule}
						class="inline-flex items-center gap-1 rounded-md border border-input bg-background px-3 py-1.5 text-xs font-medium shadow-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
					>
						<PlusIcon class="size-3.5" />
						{$t('settings.autoLabels.add')}
					</button>
					<button
						type="button"
						onclick={saveAutoLabels}
						disabled={!autoLabelsDirty || autoLabelsBusy}
						class="inline-flex items-center gap-1 rounded-md bg-foreground px-3 py-1.5 text-xs font-medium text-background shadow-sm transition-colors hover:bg-foreground/90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
					>
						{$t('settings.autoLabels.save')}
					</button>
				</div>
			</section>
		</Tabs.Content>

		<Tabs.Content value="templates" class="flex flex-col gap-4">
			<TemplatesSection />
		</Tabs.Content>

		<Tabs.Content value="project" class="flex flex-col gap-4">
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.project.bugLabelsHeading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.project.bugLabelsDescription')}</p>
				</div>
				{#if labelsStore.items.length === 0}
					<p class="text-sm text-muted-foreground">{$t('settings.project.bugLabelsEmpty')}</p>
				{:else}
					<LabelPicker value={settingsStore.bugLabelIds} onValueChange={onBugLabelsChange} />
				{/if}
			</section>

			<ProjectSuggestionsSection />
		</Tabs.Content>

		<Tabs.Content value="troiki">
			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex items-start justify-between gap-3">
					<div class="flex flex-col gap-0.5">
						<h2 class="text-sm font-semibold">{$t('settings.troiki.heading')}</h2>
						<p class="text-xs text-muted-foreground">{$t('settings.troiki.description')}</p>
					</div>
					<Switch
						checked={settingsStore.troikiEnabled}
						onCheckedChange={setTroikiEnabled}
						aria-label={$t('settings.troiki.toggle')}
					/>
				</div>
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

		<Tabs.Content value="security" class="flex flex-col gap-4">
			<TwoFactorSection available={totpAvailable} />
			<SessionsSection />
		</Tabs.Content>

		<Tabs.Content value="api" class="flex flex-col gap-4">
			<ApiTokensSection />
		</Tabs.Content>

		<Tabs.Content value="backup" class="flex flex-col gap-4">
			<BackupRestoreSection />
		</Tabs.Content>

		<Tabs.Content value="logs" class="flex flex-col gap-4">
			<LogsSection />
		</Tabs.Content>
	</Tabs.Root>
</div>
