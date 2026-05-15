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
	import { taskSelectionStore } from '$lib/stores/taskSelection.svelte';
	import CheckSquareIcon from 'phosphor-svelte/lib/CheckSquare';
	import { toast } from 'svelte-sonner';
	import ContextDialog from '$lib/components/dialog/ContextDialog.svelte';
	import TroikiTriggerIcon from './TroikiTriggerIcon.svelte';
	import { t } from '$lib/i18n';

	const TITLE_KEYS: Record<string, string> = {
		'/today': 'nav.today',
		'/tomorrow': 'nav.tomorrow',
		'/inbox': 'nav.inbox',
		'/week': 'nav.thisWeek',
		'/next-week': 'nav.nextWeek',
		'/search': 'nav.search'
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
	const pageTitle = $derived.by(() => {
		if (isTaskPage) return null;
		const key = TITLE_KEYS[page.url.pathname];
		return key ? $t(key) : viewFilterStore.title;
	});
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
			const message = err instanceof Error ? err.message : $t('topbar.failedSetContext');
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
				aria-label={$t('topbar.search')}
			>
				<MagnifyingGlassIcon class="size-4 shrink-0" />
				<span class="flex-1 text-left">{$t('topbar.search')}</span>
			</button>
			<Button
				variant="ghost"
				size="icon-sm"
				class="shrink-0 md:hidden"
				onclick={() => (mobileSearchOpen = false)}
				aria-label={$t('topbar.closeSearch')}
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
					aria-label={$t('topbar.openMenu')}
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
					aria-label={$t('sidebar.expand')}
					title={$t('sidebar.expand')}
				>
					<SidebarSimpleIcon class="size-4" />
				</Button>
			{/if}
			<div
				class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto md:flex-wrap"
				role="group"
				aria-label={$t('topbar.activeContextFilter')}
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
				{@render chip(null, $t('topbar.all'))}
				{#each contextsStore.items as ctx (ctx.id)}
					{@render chip(ctx.id, ctx.name, ctx.color)}
				{/each}
				<button
					type="button"
					onclick={() => (contextDialogOpen = true)}
					disabled={contextsLocked}
					class="inline-flex size-7 shrink-0 items-center justify-center rounded-full border border-dashed border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-transparent disabled:hover:text-muted-foreground"
					aria-label={$t('topbar.addContext')}
					title={$t('topbar.addContext')}
				>
					<PlusIcon class="size-3.5" />
				</button>
			</div>
			<!-- Desktop search input -->
			<button
				type="button"
				onclick={handleSearch}
				class="group/search hidden h-9 w-full max-w-xs items-center gap-2 rounded-md border border-border bg-muted/40 px-3 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 md:inline-flex"
				aria-label={$t('topbar.search')}
			>
				<MagnifyingGlassIcon class="size-4 shrink-0" />
				<span class="flex-1 text-left">{$t('topbar.search')}</span>
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
				aria-label={$t('topbar.search')}
				title={$t('topbar.search')}
			>
				<MagnifyingGlassIcon class="size-4" />
			</Button>
		{/if}
		<Button
			variant="ghost"
			size="icon-sm"
			onclick={() => (taskSelectionStore.mode ? taskSelectionStore.disable() : taskSelectionStore.enable())}
			aria-pressed={taskSelectionStore.mode}
			aria-label={$t('topbar.toggleSelect')}
			title={$t('topbar.toggleSelect')}
			class={taskSelectionStore.mode ? 'bg-accent text-foreground' : ''}
		>
			<CheckSquareIcon class="size-4" />
		</Button>
		<Button
			variant="secondary"
			size="icon-sm"
			onclick={() => onQuickAdd?.()}
			class="bg-muted-foreground/15 text-foreground hover:bg-muted-foreground/25"
			aria-label={$t('topbar.quickAdd')}
			title={$t('topbar.quickAdd')}
		>
			<PlusIcon class="size-4" />
		</Button>
		<a
			href={resolve('/troiki')}
			aria-label={$t('topbar.troikiSystem')}
			title={$t('topbar.troikiSystem')}
			aria-current={troikiActive ? 'page' : undefined}
			class="inline-flex size-9 items-center justify-center rounded-md transition-colors focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
			class:text-muted-foreground={!troikiActive}
			class:hover:text-primary={!troikiActive}
			class:text-foreground={troikiActive}
		>
			<TroikiTriggerIcon class="size-4" />
		</a>
	</div>
</header>

<ContextDialog bind:open={contextDialogOpen} />
