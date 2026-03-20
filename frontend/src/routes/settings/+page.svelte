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
	import { actionQueue } from '$lib/sync/action-queue.svelte';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';

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
				{#if actionQueue.items.length > 0}
					<section class="mb-6">
						<div class="mb-3 flex items-center justify-between">
							<h2 class="text-xs font-medium tracking-wider uppercase text-muted-foreground">{$t('pwa.pendingActions')}</h2>
							<button
								class="rounded-md px-2 py-1 text-[11px] font-medium text-destructive transition-colors hover:bg-destructive/10"
								onclick={() => actionQueue.clear()}
							>
								{$t('pwa.clearQueue')}
							</button>
						</div>
						<div class="space-y-1">
							{#each actionQueue.items as action (action.id)}
								<div class="flex items-center justify-between rounded-md border px-3 py-2 text-sm
									{action.status === 'failed' ? 'border-destructive/30 bg-destructive/5' : 'border-border/50'}">
									<div class="min-w-0 flex-1">
										<span class="font-mono text-xs text-muted-foreground">{action.type}</span>
										{#if action.error}
											<p class="mt-0.5 truncate text-[11px] text-destructive">{action.error}</p>
										{/if}
										<p class="text-[11px] text-muted-foreground/60">{new Date(action.createdAt).toLocaleTimeString()}</p>
									</div>
									<div class="ml-2 flex shrink-0 items-center gap-1">
										{#if action.status === 'failed'}
											<button
												class="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
												title={$t('pwa.retryAction')}
												onclick={() => actionQueue.retryFailed(action.id!)}
											>
												<RefreshCwIcon class="h-3.5 w-3.5" />
											</button>
										{/if}
										<button
											class="flex h-7 w-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
											title={$t('pwa.discardAction')}
											onclick={() => actionQueue.discard(action.id!)}
										>
											<Trash2Icon class="h-3.5 w-3.5" />
										</button>
									</div>
								</div>
							{/each}
						</div>
					</section>
				{/if}
				<LogsPanel />
			</Tabs.Content>
		</Tabs.Root>
	</div>
</div>
