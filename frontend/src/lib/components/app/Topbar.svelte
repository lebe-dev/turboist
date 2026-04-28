<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Kbd from '$lib/components/ui/kbd';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import ListIcon from 'phosphor-svelte/lib/List';
	import SidebarSimpleIcon from 'phosphor-svelte/lib/SidebarSimple';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { sidebarStore } from '$lib/stores/sidebar.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { toast } from 'svelte-sonner';
	import ContextDialog from '$lib/components/dialog/ContextDialog.svelte';
	import ThemeToggle from './ThemeToggle.svelte';

	let {
		onQuickAdd,
		onMenuClick
	}: {
		onQuickAdd?: () => void;
		onMenuClick?: () => void;
	} = $props();

	let contextDialogOpen = $state(false);

	function handleSearch(): void {
		void goto(resolve('/search'));
	}

	async function selectContext(id: number | null): Promise<void> {
		try {
			await userStateStore.setActiveContextId(id);
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Failed to set context';
			toast.error(message);
		}
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
		{#if sidebarStore.collapsed}
			<Button
				variant="ghost"
				size="icon-sm"
				class="hidden md:inline-flex"
				onclick={() => sidebarStore.toggle()}
				aria-label="Expand sidebar"
				title="Expand sidebar"
			>
				<SidebarSimpleIcon class="size-5" />
			</Button>
		{/if}
		<div
			class="hidden flex-wrap items-center gap-1 md:flex"
			role="group"
			aria-label="Active context filter"
		>
			{#snippet chip(id: number | null, label: string, color?: string)}
				{@const active = userStateStore.activeContextId === id}
				<button
					type="button"
					onclick={() => selectContext(id)}
					class="inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-[12px] transition-colors"
					class:border-border={!active}
					class:bg-transparent={!active}
					class:text-muted-foreground={!active}
					class:hover:bg-muted={!active}
					class:hover:text-foreground={!active}
					class:border-primary={active}
					class:bg-primary={active}
					class:text-primary-foreground={active}
					aria-pressed={active}
				>
					{#if color}
						<span class="size-2 shrink-0 rounded-full" style={`background-color: ${color}`}></span>
					{/if}
					<span class="truncate">{label}</span>
				</button>
			{/snippet}
			{@render chip(null, 'All')}
			{#each contextsStore.items as ctx (ctx.id)}
				{@render chip(ctx.id, ctx.name, ctx.color)}
			{/each}
			<button
				type="button"
				onclick={() => (contextDialogOpen = true)}
				class="inline-flex size-7 items-center justify-center rounded-full border border-dashed border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
				aria-label="Add context"
				title="Add context"
			>
				<PlusIcon class="size-3.5" />
			</button>
		</div>
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
	</div>
</header>

<ContextDialog bind:open={contextDialogOpen} />
