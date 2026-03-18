<script lang="ts">
	import { goto } from '$app/navigation';
	import { setMode, userPrefersMode } from 'mode-watcher';
	import { t, locale } from 'svelte-intl-precompile';
	import { availableLocales } from '$lib/i18n';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import MonitorIcon from '@lucide/svelte/icons/monitor';
	import * as Tabs from '$lib/components/ui/tabs';
	import LogsPanel from '$lib/components/LogsPanel.svelte';

	const themes = [
		{ value: 'light' as const, key: 'settings.theme.light', icon: SunIcon },
		{ value: 'dark' as const, key: 'settings.theme.dark', icon: MoonIcon },
		{ value: 'system' as const, key: 'settings.theme.system', icon: MonitorIcon }
	];

	const localeLabels: Record<string, string> = {
		en: 'English',
		ru: 'Русский'
	};
</script>

<div class="flex h-full flex-col">
	<header class="flex h-12 shrink-0 items-center gap-3 border-b border-border/50 px-4">
		<button
			class="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
			onclick={() => goto('/')}
			aria-label="Back"
		>
			<ArrowLeftIcon class="h-4 w-4" />
		</button>
		<h1 class="text-sm font-semibold tracking-wide text-foreground">{$t('settings.title')}</h1>
	</header>

	<div class="flex-1 overflow-y-auto px-4 py-6 md:px-6">
		<Tabs.Root value="general">
			<Tabs.List class="mb-6">
				<Tabs.Trigger value="general">{$t('settings.general')}</Tabs.Trigger>
				<Tabs.Trigger value="logs">{$t('settings.logs')}</Tabs.Trigger>
			</Tabs.List>

			<Tabs.Content value="general">
				<section>
					<h2 class="mb-3 text-xs font-medium tracking-wider uppercase text-muted-foreground">{$t('settings.appearance')}</h2>
					<div class="grid grid-cols-3 gap-2">
						{#each themes as theme}
							<button
								class="flex flex-col items-center gap-2 rounded-lg border px-3 py-4 text-sm transition-colors
									{userPrefersMode.current === theme.value
										? 'border-primary/50 bg-primary/5 text-foreground'
										: 'border-border/50 text-muted-foreground hover:bg-accent/30'}"
								onclick={() => setMode(theme.value)}
							>
								<theme.icon class="h-5 w-5" />
								{$t(theme.key)}
							</button>
						{/each}
					</div>
				</section>

				<section class="mt-8">
					<h2 class="mb-3 text-xs font-medium tracking-wider uppercase text-muted-foreground">{$t('settings.language')}</h2>
					<div class="grid grid-cols-2 gap-2">
						{#each availableLocales as loc}
							<button
								class="flex items-center justify-center gap-2 rounded-lg border px-3 py-4 text-sm transition-colors
									{$locale === loc
										? 'border-primary/50 bg-primary/5 text-foreground'
										: 'border-border/50 text-muted-foreground hover:bg-accent/30'}"
								onclick={() => locale.set(loc)}
							>
								{localeLabels[loc] ?? loc}
							</button>
						{/each}
					</div>
				</section>
			</Tabs.Content>

			<Tabs.Content value="logs">
				<LogsPanel />
			</Tabs.Content>
		</Tabs.Root>
	</div>
</div>
