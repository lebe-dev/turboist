<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import * as Kbd from '$lib/components/ui/kbd';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import XIcon from 'phosphor-svelte/lib/X';
	import ListIcon from 'phosphor-svelte/lib/List';
	import SidebarSimpleIcon from 'phosphor-svelte/lib/SidebarSimple';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { sidebarStore } from '$lib/stores/sidebar.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { viewFilterStore } from '$lib/stores/viewFilter.svelte';
	import { toast } from 'svelte-sonner';
	import ContextDialog from '$lib/components/dialog/ContextDialog.svelte';
	import TroikiTriggerIcon from './TroikiTriggerIcon.svelte';

	const STATIC_TITLES: Record<string, string> = {
		'/today': 'Today',
		'/tomorrow': 'Tomorrow',
		'/inbox': 'Inbox',
		'/week': 'This week',
		'/backlog': 'Backlog',
		'/next-week': 'Next week',
		'/search': 'Search',
		'/troiki': 'Troiki',
		'/settings': 'Settings',
	};

	let {
		onQuickAdd,
		onMenuClick
	}: {
		onQuickAdd?: () => void;
		onMenuClick?: () => void;
	} = $props();

	let contextDialogOpen = $state(false);
	let mobileSearchOpen = $state(false);

	const isTaskPage = $derived(page.url.pathname.startsWith('/task/'));
	const pageTitle = $derived(
		isTaskPage ? null : (STATIC_TITLES[page.url.pathname] ?? viewFilterStore.title)
	);
	const contextsLocked = $derived(page.url.pathname === '/inbox');
	const troikiActive = $derived(page.url.pathname === '/troiki');

	function handleSearch(): void {
		mobileSearchOpen = false;
		void goto(resolve('/search'));
	}

	async function selectContext(id: number | null): Promise<void> {
		if (contextsLocked) return;
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
		{#if mobileSearchOpen}
			<!-- Mobile: expanded search input -->
			<button
				type="button"
				onclick={handleSearch}
				class="group/search inline-flex h-9 w-full items-center gap-2 rounded-md border border-border bg-muted/40 px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 md:hidden"
				aria-label="Search"
			>
				<MagnifyingGlassIcon class="size-4 shrink-0" />
				<span class="flex-1 text-left">Search</span>
			</button>
			<Button
				variant="ghost"
				size="icon-sm"
				class="shrink-0 md:hidden"
				onclick={() => (mobileSearchOpen = false)}
				aria-label="Close search"
			>
				<XIcon class="size-4" />
			</Button>
		{/if}

		<div class="flex min-w-0 flex-1 items-center gap-2" class:hidden={mobileSearchOpen}>
			{#if onMenuClick}
				<Button
					variant="ghost"
					size="icon-sm"
					class="shrink-0 md:hidden"
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
					class="hidden shrink-0 md:inline-flex"
					onclick={() => sidebarStore.toggle()}
					aria-label="Expand sidebar"
					title="Expand sidebar"
				>
					<SidebarSimpleIcon class="size-5" />
				</Button>
			{/if}
			<div
				class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto md:flex-wrap"
				role="group"
				aria-label="Active context filter"
			>
				{#snippet chip(id: number | null, label: string, color?: string)}
					{@const active = userStateStore.activeContextId === id}
					<button
						type="button"
						onclick={() => selectContext(id)}
						disabled={contextsLocked}
						class="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border border-border px-2.5 text-[12px] transition-colors disabled:cursor-not-allowed disabled:opacity-60"
						class:bg-transparent={!active}
						class:text-muted-foreground={!active || contextsLocked}
						class:hover:bg-muted={!active && !contextsLocked}
						class:hover:text-foreground={!active && !contextsLocked}
						class:bg-muted={active}
						class:text-foreground={active && !contextsLocked}
						aria-pressed={active}
					>
						{#if color}
							<span
								class="size-2 shrink-0 rounded-full"
								style={contextsLocked ? undefined : `background-color: ${color}`}
								class:bg-muted-foreground={contextsLocked}
							></span>
						{/if}
						<span class="truncate">{label}</span>
					</button>
				{/snippet}
				{#if pageTitle}
					<span class="mr-1 shrink-0 text-[13px] font-semibold text-foreground">{pageTitle}</span>
				{/if}
				{@render chip(null, 'All')}
				{#each contextsStore.items as ctx (ctx.id)}
					{@render chip(ctx.id, ctx.name, ctx.color)}
				{/each}
				<button
					type="button"
					onclick={() => (contextDialogOpen = true)}
					disabled={contextsLocked}
					class="inline-flex size-7 shrink-0 items-center justify-center rounded-full border border-dashed border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-transparent disabled:hover:text-muted-foreground"
					aria-label="Add context"
					title="Add context"
				>
					<PlusIcon class="size-3.5" />
				</button>
			</div>
			<!-- Desktop search input -->
			<button
				type="button"
				onclick={handleSearch}
				class="group/search hidden h-9 w-full max-w-xs items-center gap-2 rounded-md border border-border bg-muted/40 px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 md:inline-flex"
				aria-label="Search"
			>
				<MagnifyingGlassIcon class="size-4 shrink-0" />
				<span class="flex-1 text-left">Search</span>
				<Kbd.Kbd>/</Kbd.Kbd>
			</button>
		</div>
	</div>
	<div class="flex shrink-0 items-center gap-1 sm:gap-1.5">
		<!-- Mobile search icon -->
		{#if !mobileSearchOpen}
			<Button
				variant="ghost"
				size="icon-sm"
				class="md:hidden"
				onclick={() => (mobileSearchOpen = true)}
				aria-label="Search"
				title="Search"
			>
				<MagnifyingGlassIcon class="size-4" />
			</Button>
		{/if}
		<Button
			variant="secondary"
			size="icon-sm"
			onclick={() => onQuickAdd?.()}
			class="bg-muted-foreground/15 text-foreground hover:bg-muted-foreground/25"
			aria-label="Quick add (Q)"
			title="Quick add (Q)"
		>
			<PlusIcon class="size-4" />
		</Button>
		<a
			href={resolve('/troiki')}
			aria-label="Troiki System"
			title="Troiki System"
			aria-current={troikiActive ? 'page' : undefined}
			class="inline-flex size-9 items-center justify-center rounded-md transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
			class:text-muted-foreground={!troikiActive}
			class:hover:text-primary={!troikiActive}
			class:text-primary={troikiActive}
		>
			<TroikiTriggerIcon class="size-4" />
		</a>
	</div>
</header>

<ContextDialog bind:open={contextDialogOpen} />
