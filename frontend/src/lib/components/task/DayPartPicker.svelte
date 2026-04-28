<script lang="ts">
	import type { DayPart } from '$lib/api/types';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import SunDimIcon from 'phosphor-svelte/lib/SunDim';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import ClockIcon from 'phosphor-svelte/lib/Clock';
	import type { Component } from 'svelte';

	let { value = $bindable<DayPart>('none') }: { value?: DayPart } = $props();

	const OPTIONS: Array<{ part: DayPart; label: string; icon: Component }> = [
		{ part: 'none', label: 'Anytime', icon: ClockIcon as unknown as Component },
		{ part: 'morning', label: 'Morning', icon: SunIcon as unknown as Component },
		{ part: 'afternoon', label: 'Afternoon', icon: SunDimIcon as unknown as Component },
		{ part: 'evening', label: 'Evening', icon: MoonIcon as unknown as Component }
	];

	const current = $derived(OPTIONS.find((o) => o.part === value) ?? OPTIONS[0]);
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="inline-flex items-center gap-1 rounded px-1.5 py-1 text-xs hover:bg-muted"
		aria-label="Day part"
	>
		{@const Icon = current.icon}
		<Icon class="size-3.5" />
		<span>{current.label}</span>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content>
		{#each OPTIONS as opt (opt.part)}
			{@const Icon = opt.icon}
			<DropdownMenu.Item onSelect={() => (value = opt.part)}>
				<Icon class="size-3.5" />
				<span>{opt.label}</span>
			</DropdownMenu.Item>
		{/each}
	</DropdownMenu.Content>
</DropdownMenu.Root>
