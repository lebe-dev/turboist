<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Kbd from '$lib/components/ui/kbd';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import ListIcon from 'phosphor-svelte/lib/List';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import ThemeToggle from './ThemeToggle.svelte';
	import UserMenu from './UserMenu.svelte';

	let {
		onQuickAdd,
		onMenuClick
	}: {
		onQuickAdd?: () => void;
		onMenuClick?: () => void;
	} = $props();

	function handleSearch(): void {
		void goto(resolve('/search'));
	}
</script>

<header
	class="flex h-14 shrink-0 items-center justify-between gap-2 border-b border-border bg-background/80 px-3 backdrop-blur-sm sm:gap-3 sm:px-4"
>
	<div class="flex min-w-0 flex-1 items-center gap-2">
		{#if onMenuClick}
			<Button
				variant="ghost"
				size="icon-sm"
				class="md:hidden"
				onclick={() => onMenuClick?.()}
				aria-label="Open menu"
			>
				<ListIcon class="size-5" />
			</Button>
		{/if}
		<button
			type="button"
			onclick={handleSearch}
			class="group/search inline-flex h-9 w-full max-w-xs items-center gap-2 rounded-md border border-border bg-muted/40 px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
			aria-label="Search"
		>
			<MagnifyingGlassIcon class="size-4 shrink-0" />
			<span class="flex-1 text-left">Search</span>
			<Kbd.Kbd class="hidden sm:inline-flex">/</Kbd.Kbd>
		</button>
	</div>
	<div class="flex shrink-0 items-center gap-1 sm:gap-1.5">
		<Button
			variant="ghost"
			size="sm"
			onclick={() => onQuickAdd?.()}
			class="gap-1.5"
			aria-label="Quick add"
			title="Quick add"
		>
			<PlusIcon class="size-4" />
			<span class="hidden sm:inline">Quick add</span>
			<Kbd.Kbd class="ml-1 hidden sm:inline-flex">Q</Kbd.Kbd>
		</Button>
		<ThemeToggle />
		<UserMenu />
	</div>
</header>
