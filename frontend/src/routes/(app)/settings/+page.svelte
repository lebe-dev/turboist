<script lang="ts">
	import { userPrefersMode, setMode } from 'mode-watcher';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import MonitorIcon from 'phosphor-svelte/lib/Monitor';
	import CheckIcon from 'phosphor-svelte/lib/Check';
	import SignOutIcon from 'phosphor-svelte/lib/SignOut';
	import QuestionIcon from 'phosphor-svelte/lib/Question';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as HoverCard from '$lib/components/ui/hover-card';
	import ApiTokensSection from '$lib/components/settings/ApiTokensSection.svelte';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { appSettingsStore } from '$lib/stores/appSettings.svelte';
	import type { AutoLabelRule } from '$lib/api/types';
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
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

	let autoLabelsDraft = $state<AutoLabelRule[]>(
		appSettingsStore.autoLabels.map((r) => ({ ...r, labelIds: [...r.labelIds] }))
	);
	let autoLabelsBusy = $state(false);

	const allLabels = $derived([...labelsStore.favourites, ...labelsStore.rest]);
	const labelNameById = $derived(new Map(allLabels.map((l) => [l.id, l.name])));

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

	function addAutoLabelRule(): void {
		autoLabelsDraft = [...autoLabelsDraft, { mask: '', labelIds: [], ignoreCase: true }];
	}

	function removeAutoLabelRule(idx: number): void {
		autoLabelsDraft = autoLabelsDraft.filter((_, i) => i !== idx);
	}

	function toggleRuleLabel(ruleIdx: number, labelId: number): void {
		const rule = autoLabelsDraft[ruleIdx];
		if (!rule) return;
		const ids = rule.labelIds.includes(labelId)
			? rule.labelIds.filter((id) => id !== labelId)
			: [...rule.labelIds, labelId];
		autoLabelsDraft = autoLabelsDraft.map((r, i) => (i === ruleIdx ? { ...r, labelIds: ids } : r));
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

	<Tabs.Root value="general" class="flex flex-col gap-4">
		<Tabs.List variant="line">
			<Tabs.Trigger value="general">{$t('settings.tabs.general')}</Tabs.Trigger>
			<Tabs.Trigger value="labels">{$t('settings.tabs.labels')}</Tabs.Trigger>
			<Tabs.Trigger value="project">{$t('settings.tabs.project')}</Tabs.Trigger>
			<Tabs.Trigger value="privacy">{$t('settings.tabs.privacy')}</Tabs.Trigger>
			<Tabs.Trigger value="session">{$t('settings.tabs.session')}</Tabs.Trigger>
			<Tabs.Trigger value="api">{$t('settings.tabs.api')}</Tabs.Trigger>
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

			<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
				<div class="flex flex-col gap-0.5">
					<h2 class="text-sm font-semibold">{$t('settings.autoLabels.heading')}</h2>
					<p class="text-xs text-muted-foreground">{$t('settings.autoLabels.description')}</p>
				</div>

				{#if autoLabelsDraft.length === 0}
					<p class="text-sm text-muted-foreground">{$t('settings.autoLabels.empty')}</p>
				{:else}
					<div class="flex flex-col gap-2">
						<div class="grid grid-cols-[1fr_1fr_auto_auto] items-center gap-2 px-1 text-[11px] font-medium text-muted-foreground">
							<span>{$t('settings.autoLabels.mask')}</span>
							<span>{$t('settings.autoLabels.labels')}</span>
							<span>{$t('settings.autoLabels.ignoreCase')}</span>
							<span class="sr-only">{$t('settings.autoLabels.remove')}</span>
						</div>
						{#each autoLabelsDraft as rule, idx (idx)}
							{@const selectedNames = rule.labelIds
								.map((id) => labelNameById.get(id))
								.filter((n): n is string => !!n)}
							<div class="grid grid-cols-[1fr_1fr_auto_auto] items-center gap-2">
								<input
									type="text"
									bind:value={rule.mask}
									placeholder={$t('settings.autoLabels.maskPlaceholder')}
									class="rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
								/>
								<DropdownMenu.Root>
									<DropdownMenu.Trigger
										class="flex items-center justify-between gap-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
									>
										<span class="truncate text-left {selectedNames.length === 0 ? 'text-muted-foreground' : ''}">
											{selectedNames.length === 0
												? $t('settings.autoLabels.labelsPlaceholder')
												: selectedNames.join(', ')}
										</span>
										<CaretDownIcon class="size-3.5 shrink-0 text-muted-foreground" />
									</DropdownMenu.Trigger>
									<DropdownMenu.Content class="max-h-60 w-56 overflow-auto">
										{#if allLabels.length === 0}
											<div class="px-2 py-1.5 text-xs text-muted-foreground">
												{$t('settings.autoLabels.noLabelsAvailable')}
											</div>
										{:else}
											{#each allLabels as label (label.id)}
												<DropdownMenu.CheckboxItem
													checked={rule.labelIds.includes(label.id)}
													onCheckedChange={() => toggleRuleLabel(idx, label.id)}
													closeOnSelect={false}
												>
													{label.name}
												</DropdownMenu.CheckboxItem>
											{/each}
										{/if}
									</DropdownMenu.Content>
								</DropdownMenu.Root>
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

		<Tabs.Content value="api" class="flex flex-col gap-4">
			<ApiTokensSection />
		</Tabs.Content>
	</Tabs.Root>
</div>
