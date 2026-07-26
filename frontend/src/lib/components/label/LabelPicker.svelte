<script lang="ts">
	import { Popover as PopoverPrimitive } from 'bits-ui';
	import { tick } from 'svelte';
	import * as Sheet from '$lib/components/ui/sheet';
	import { IsMobile } from '$lib/hooks';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { isLabelVisible } from '$lib/utils/visibility';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import XIcon from 'phosphor-svelte/lib/X';
	import { t } from '$lib/i18n';

	let {
		value = $bindable<number[]>([]),
		onValueChange
	}: {
		value?: number[];
		onValueChange?: (value: number[]) => void;
	} = $props();

	const isMobile = new IsMobile();

	let menuOpen = $state(false);
	let query = $state('');
	let searchInput = $state<HTMLInputElement | null>(null);

	const allLabels = $derived.by(() => {
		const byName = (a: { name: string }, b: { name: string }) =>
			a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
		return [...labelsStore.favourites, ...labelsStore.rest]
			.filter((l) => isLabelVisible(l, settingsStore.publicView))
			.toSorted(byName);
	});
	const filteredLabels = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return allLabels;
		return allLabels.filter((l) => l.name.toLowerCase().includes(q));
	});

	$effect(() => {
		if (menuOpen) {
			tick().then(() => searchInput?.focus());
		} else {
			query = '';
		}
	});

	function toggle(id: number): void {
		value = value.includes(id) ? value.filter((x) => x !== id) : [...value, id];
		onValueChange?.(value);
	}
</script>

{#if allLabels.length > 0}
	{#snippet search()}
		<div class="flex items-center gap-2 border-b border-border px-2.5 py-1.5">
			<MagnifyingGlassIcon class="size-3.5 text-muted-foreground" />
			<input
				bind:this={searchInput}
				bind:value={query}
				type="text"
				placeholder={$t('label.picker.searchPlaceholder')}
				class="w-full bg-transparent {isMobile.current ? 'h-7 text-sm' : 'h-6 text-xs'} outline-none placeholder:text-muted-foreground"
				onkeydown={(e) => {
					if (e.key === 'Escape') {
						e.stopPropagation();
						menuOpen = false;
					}
				}}
			/>
		</div>
	{/snippet}
	{#snippet options()}
		{#each filteredLabels as label (label.id)}
			{@const active = value.includes(label.id)}
			<button
				type="button"
				onclick={() => toggle(label.id)}
				class="inline-flex items-center rounded-md text-left transition-colors {isMobile.current
					? 'gap-3 px-3 py-3 text-sm'
					: 'gap-2 px-2 py-1.5 text-xs'}"
				class:bg-accent={active}
				class:text-accent-foreground={active}
				class:hover:bg-accent={!active}
			>
				{#if label.color}
					<span
						class="rounded-full {isMobile.current ? 'size-3' : 'size-2'}"
						style={`background-color: ${label.color}`}
					></span>
				{/if}
				<span class="flex-1 truncate">{label.name}</span>
				{#if active}
					<XIcon class="opacity-60 {isMobile.current ? 'size-4' : 'size-3'}" />
				{/if}
			</button>
		{/each}
		{#if filteredLabels.length === 0}
			<div class="px-2 py-3 text-center text-xs text-muted-foreground">
				{$t('label.picker.noMatches')}
			</div>
		{/if}
	{/snippet}

	{#if isMobile.current}
		<button
			type="button"
			onclick={() => (menuOpen = !menuOpen)}
			aria-expanded={menuOpen}
			class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground aria-expanded:bg-accent"
		>
			<TagIcon class="size-3.5" />
			<span>{$t('common.labels')}</span>
			{#if value.length > 0}
				<span class="rounded-full bg-primary/15 px-1.5 text-[10px] font-semibold text-primary">
					{value.length}
				</span>
			{/if}
		</button>
		<Sheet.Root bind:open={menuOpen}>
			<Sheet.Content side="bottom" class="max-h-[80vh] overflow-y-auto rounded-t-lg p-3">
				<Sheet.Header class="px-2 pb-2 pt-0">
					<Sheet.Title>{$t('common.labels')}</Sheet.Title>
				</Sheet.Header>
				{@render search()}
				<div class="flex flex-col gap-2 pb-4 pt-2">
					{@render options()}
				</div>
			</Sheet.Content>
		</Sheet.Root>
	{:else}
		<PopoverPrimitive.Root bind:open={menuOpen}>
			<PopoverPrimitive.Trigger>
				{#snippet child({ props })}
					<button
						{...props}
						type="button"
						class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground data-[state=open]:bg-accent"
					>
						<TagIcon class="size-3.5" />
						<span>{$t('common.labels')}</span>
						{#if value.length > 0}
							<span class="rounded-full bg-primary/15 px-1.5 text-[10px] font-semibold text-primary">
								{value.length}
							</span>
						{/if}
					</button>
				{/snippet}
			</PopoverPrimitive.Trigger>
			<PopoverPrimitive.Portal>
				<PopoverPrimitive.Content
					align="start"
					sideOffset={4}
					class="z-[60] flex max-h-64 w-56 flex-col rounded-md border border-border bg-popover shadow-lg outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
				>
					{@render search()}
					<div class="flex flex-col gap-1 overflow-y-auto p-2">
						{@render options()}
					</div>
				</PopoverPrimitive.Content>
			</PopoverPrimitive.Portal>
		</PopoverPrimitive.Root>
	{/if}
{/if}
