<script lang="ts">
	import { userPrefersMode, setMode } from 'mode-watcher';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import MonitorIcon from 'phosphor-svelte/lib/Monitor';

	type ThemeMode = 'light' | 'dark' | 'system';

	type ThemeOption = {
		value: ThemeMode;
		label: string;
		description: string;
		icon: typeof SunIcon;
	};

	const themeOptions: ThemeOption[] = [
		{
			value: 'light',
			label: 'Light',
			description: 'Always use the light theme',
			icon: SunIcon
		},
		{
			value: 'dark',
			label: 'Dark',
			description: 'Always use the dark theme',
			icon: MoonIcon
		},
		{
			value: 'system',
			label: 'System',
			description: 'Match your operating system preference',
			icon: MonitorIcon
		}
	];

	const current = $derived(userPrefersMode.current);
</script>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-6 px-4 py-8 sm:px-6">
	<header class="flex flex-col gap-1">
		<h1 class="text-xl font-semibold tracking-tight">Settings</h1>
		<p class="text-sm text-muted-foreground">Personalise how Turboist looks and behaves.</p>
	</header>

	<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">Theme</h2>
			<p class="text-xs text-muted-foreground">Choose between light, dark, or matching your system.</p>
		</div>
		<div class="grid gap-2 sm:grid-cols-3" role="radiogroup" aria-label="Theme">
			{#each themeOptions as option (option.value)}
				{@const Icon = option.icon}
				{@const active = current === option.value}
				<button
					type="button"
					role="radio"
					aria-checked={active}
					onclick={() => setMode(option.value)}
					class="flex flex-col items-start gap-2 rounded-md border p-3 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
					class:border-primary={active}
					class:bg-muted={active}
					class:border-border={!active}
				>
					<span class="flex items-center gap-2">
						<Icon class="size-4" weight={active ? 'fill' : 'regular'} />
						<span class="text-sm font-medium">{option.label}</span>
					</span>
					<span class="text-xs text-muted-foreground">{option.description}</span>
				</button>
			{/each}
		</div>
	</section>
</div>
