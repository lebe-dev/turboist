<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Kbd from '$lib/components/ui/kbd';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import ThemeToggle from './ThemeToggle.svelte';
	import UserMenu from './UserMenu.svelte';

	let {
		onQuickAdd
	}: {
		onQuickAdd?: () => void;
	} = $props();

	function handleSearch(): void {
		void goto(resolve('/search'));
	}
</script>

<header
	class="flex h-12 shrink-0 items-center justify-between border-b border-border bg-background px-3"
>
	<div class="flex items-center gap-2">
		<Button variant="default" size="sm" onclick={() => onQuickAdd?.()} class="gap-2">
			<PlusIcon class="size-4" />
			<span>Quick add</span>
			<Kbd.Kbd class="ml-2">Q</Kbd.Kbd>
		</Button>
		<Button variant="ghost" size="sm" onclick={handleSearch} class="gap-2">
			<MagnifyingGlassIcon class="size-4" />
			<span>Search</span>
			<Kbd.Kbd class="ml-2">/</Kbd.Kbd>
		</Button>
	</div>
	<div class="flex items-center gap-1">
		<ThemeToggle />
		<UserMenu />
	</div>
</header>
