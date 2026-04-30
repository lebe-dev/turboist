<script lang="ts">
	import type { Component } from 'svelte';
	import type { DayPart } from '$lib/api/types';
	import type { DayPartInterval } from '$lib/utils/viewGroup';
	import { iconFor } from './dayPartIcon';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import ArrowRightIcon from 'phosphor-svelte/lib/ArrowRight';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import ClockIcon from 'phosphor-svelte/lib/Clock';

	let {
		part,
		label,
		interval,
		count,
		onBulkMove
	}: {
		part: DayPart;
		label: string;
		interval: DayPartInterval | null;
		count: number;
		onBulkMove?: (targetPart: DayPart) => void;
	} = $props();

	const Icon = $derived(iconFor(part));

	function fmtHour(h: number): string {
		return `${h}:00`;
	}

	const ALL_PARTS: Array<{ part: DayPart; label: string; icon: Component }> = [
		{ part: 'morning', label: 'Morning', icon: SunHorizonIcon as unknown as Component },
		{ part: 'afternoon', label: 'Afternoon', icon: SunIcon as unknown as Component },
		{ part: 'evening', label: 'Evening', icon: MoonIcon as unknown as Component },
		{ part: 'none', label: 'Anytime', icon: ClockIcon as unknown as Component }
	];

	const targetParts = $derived(ALL_PARTS.filter((p) => p.part !== part));
</script>

<div class="flex items-center gap-2 px-3 pb-1 text-xs font-semibold uppercase tracking-wide">
	<Icon class="size-3.5 text-muted-foreground" />
	<span class="text-muted-foreground">{label}</span>
	{#if interval}
		<span class="font-normal normal-case text-muted-foreground/70">
			{fmtHour(interval.start)}–{fmtHour(interval.end)}
		</span>
	{/if}
	<span class="font-normal text-muted-foreground/70">{count}</span>

	{#if onBulkMove && count > 0}
		<div class="ml-auto">
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<button
							{...props}
							type="button"
							title="Move all tasks to another phase"
							class="inline-flex size-5 items-center justify-center rounded text-muted-foreground/40 transition-colors hover:text-muted-foreground"
						>
							<ArrowRightIcon class="size-3.5" />
						</button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="min-w-[12rem]">
					{#each targetParts as opt (opt.part)}
						{@const TargetIcon = opt.icon}
						<DropdownMenu.Item onclick={() => onBulkMove(opt.part)}>
							<TargetIcon class="size-4" />
							Move all to {opt.label}
						</DropdownMenu.Item>
					{/each}
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	{/if}
</div>
