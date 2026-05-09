<script lang="ts">
	import type { DayPart } from '$lib/api/types';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import ClockIcon from 'phosphor-svelte/lib/Clock';
	import type { Component } from 'svelte';
	import { t } from '$lib/i18n';

	let { value = $bindable<DayPart>('none') }: { value?: DayPart } = $props();

	const OPTIONS: Array<{ part: DayPart; labelKey: string; icon: Component; color: string }> = [
		{ part: 'morning', labelKey: 'task.dayPart.morning', icon: SunHorizonIcon as unknown as Component, color: 'text-orange-400' },
		{ part: 'afternoon', labelKey: 'task.dayPart.afternoon', icon: SunIcon as unknown as Component, color: 'text-yellow-400' },
		{ part: 'evening', labelKey: 'task.dayPart.evening', icon: MoonIcon as unknown as Component, color: 'text-indigo-400' },
		{ part: 'none', labelKey: 'task.dayPart.anytime', icon: ClockIcon as unknown as Component, color: 'text-foreground' }
	];
</script>

<div
	class="inline-flex w-fit items-center gap-0.5 rounded-md border border-border bg-background p-0.5"
	role="radiogroup"
	aria-label={$t('task.dayPart.ariaLabel')}
>
	{#each OPTIONS as opt (opt.part)}
		{@const Icon = opt.icon}
		{@const active = value === opt.part}
		{@const label = $t(opt.labelKey)}
		<button
			type="button"
			role="radio"
			aria-checked={active}
			aria-label={label}
			title={label}
			onclick={() => (value = opt.part)}
			class="inline-flex h-7 w-7 items-center justify-center rounded-[5px] transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50
				{active ? `bg-accent ${opt.color}` : 'text-muted-foreground hover:bg-accent hover:text-foreground'}"
		>
			<Icon class="size-3.5" />
		</button>
	{/each}
</div>
