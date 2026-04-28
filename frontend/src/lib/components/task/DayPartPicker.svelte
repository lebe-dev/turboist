<script lang="ts">
	import type { DayPart } from '$lib/api/types';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import ClockIcon from 'phosphor-svelte/lib/Clock';
	import type { Component } from 'svelte';

	let {
		value = $bindable<DayPart>('none'),
		compact = false
	}: { value?: DayPart; compact?: boolean } = $props();

	const OPTIONS: Array<{ part: DayPart; label: string; icon: Component }> = [
		{ part: 'morning', label: 'Morning', icon: SunHorizonIcon as unknown as Component },
		{ part: 'afternoon', label: 'Afternoon', icon: SunIcon as unknown as Component },
		{ part: 'evening', label: 'Evening', icon: MoonIcon as unknown as Component },
		{ part: 'none', label: 'Anytime', icon: ClockIcon as unknown as Component }
	];

	const labelClass = $derived(compact ? 'sr-only' : 'sr-only sm:not-sr-only');
</script>

<div
	class="inline-flex w-fit items-center gap-0.5 rounded-md border border-border bg-background p-0.5"
	role="radiogroup"
	aria-label="Day part"
>
	{#each OPTIONS as opt (opt.part)}
		{@const Icon = opt.icon}
		{@const active = value === opt.part}
		<button
			type="button"
			role="radio"
			aria-checked={active}
			aria-label={opt.label}
			title={opt.label}
			onclick={() => (value = opt.part)}
			class="inline-flex h-7 items-center gap-1.5 rounded-[5px] px-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50"
			class:bg-accent={active}
			class:text-foreground={active}
			class:text-muted-foreground={!active}
			class:hover:bg-accent={!active}
			class:hover:text-foreground={!active}
		>
			<Icon class="size-3.5" />
			<span class={labelClass}>{opt.label}</span>
		</button>
	{/each}
</div>
