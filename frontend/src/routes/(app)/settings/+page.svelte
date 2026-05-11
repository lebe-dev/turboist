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
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
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

	function toggleLabel(id: number) {
		const excluded = settingsStore.weeklyUnplannedExcludedLabelIds;
		const next = excluded.includes(id) ? excluded.filter((x) => x !== id) : [...excluded, id];
		settingsStore.setWeeklyUnplannedExcludedLabelIds(next).catch(console.error);
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
