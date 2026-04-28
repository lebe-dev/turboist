<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import LightningIcon from 'phosphor-svelte/lib/Lightning';
	import InboxIcon from 'phosphor-svelte/lib/Tray';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import StackIcon from 'phosphor-svelte/lib/Stack';
	import WarningIcon from 'phosphor-svelte/lib/Warning';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import SidebarSection from './SidebarSection.svelte';
	import ContextDialog from '$lib/components/dialog/ContextDialog.svelte';
	import LabelDialog from '$lib/components/dialog/LabelDialog.svelte';
	import ProjectDialog from '$lib/components/dialog/ProjectDialog.svelte';

	let contextDialogOpen = $state(false);
	let labelDialogOpen = $state(false);
	let projectDialogOpen = $state(false);
	let projectDialogContextId = $state<number | null>(null);

	const weekLimit = $derived(configStore.value?.weekly.limit);
	const backlogLimit = $derived(configStore.value?.backlog.limit);

	type NavItem = {
		href: string;
		label: string;
		icon: typeof InboxIcon;
		badge?: number;
		accent?: boolean;
	};

	const primaryNav = $derived<NavItem[]>([
		{ href: resolve('/inbox'), label: 'Inbox', icon: InboxIcon, accent: true },
		{ href: resolve('/today'), label: 'Today', icon: SunIcon },
		{ href: resolve('/tomorrow'), label: 'Tomorrow', icon: SunHorizonIcon },
		{ href: resolve('/week'), label: 'Week', icon: CalendarIcon, badge: weekLimit }
	]);

	const planningNav = $derived<NavItem[]>([
		{ href: resolve('/backlog'), label: 'Backlog', icon: StackIcon, badge: backlogLimit },
		{ href: resolve('/overdue'), label: 'Overdue', icon: WarningIcon },
		{ href: resolve('/search'), label: 'Search', icon: MagnifyingGlassIcon }
	]);

	function isActive(href: string): boolean {
		return page.url.pathname === href;
	}

	const labelsOrdered = $derived([...labelsStore.favourites, ...labelsStore.rest]);
</script>

{#snippet navLink(item: NavItem)}
	{@const Icon = item.icon}
	{@const active = isActive(item.href)}
	<a
		href={item.href as ReturnType<typeof resolve>}
		class="group/nav flex items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-sm transition-colors"
		class:bg-sidebar-accent={active}
		class:text-sidebar-accent-foreground={active && !item.accent}
		class:font-medium={active}
		class:text-primary={item.accent}
		class:hover:bg-sidebar-accent={!active}
		class:hover:text-sidebar-accent-foreground={!active && !item.accent}
		aria-current={active ? 'page' : undefined}
	>
		<span class="flex min-w-0 items-center gap-2.5">
			<Icon class="size-[18px] shrink-0" weight={active ? 'fill' : 'regular'} />
			<span class="truncate">{item.label}</span>
		</span>
		{#if item.badge != null}
			<span
				class="font-mono text-[11px] tabular-nums"
				class:text-primary={item.accent}
				class:text-muted-foreground={!item.accent}
			>
				{item.badge}
			</span>
		{/if}
	</a>
{/snippet}

<aside
	class="flex h-full w-64 shrink-0 flex-col overflow-y-auto border-r border-sidebar-border bg-sidebar text-sidebar-foreground"
>
	<div class="flex items-center gap-2 px-4 pb-3 pt-4">
		<span
			class="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground shadow-sm"
		>
			<LightningIcon class="size-4" weight="fill" />
		</span>
		<span class="text-[13px] font-semibold uppercase tracking-[0.18em]">Turboist</span>
	</div>

	<nav class="flex flex-col gap-0.5 px-2 pb-2" aria-label="Main">
		{#each primaryNav as item (item.href)}
			{@render navLink(item)}
		{/each}
	</nav>

	<SidebarSection title="Planning">
		{#each planningNav as item (item.href)}
			{@render navLink(item)}
		{/each}
	</SidebarSection>

	{#if projectsStore.pinned.length > 0}
		<SidebarSection title="Pinned">
			{#each projectsStore.pinned as project (project.id)}
				{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
				{@const active = isActive(href)}
				<a
					{href}
					class="flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors hover:bg-sidebar-accent"
					class:bg-sidebar-accent={active}
					class:font-medium={active}
				>
					<PushPinIcon class="size-4 shrink-0 text-amber-500" weight="fill" />
					<span class="truncate">{project.title}</span>
				</a>
			{/each}
		</SidebarSection>
	{/if}

	<SidebarSection title="Contexts" collapsible onAdd={() => (contextDialogOpen = true)}>
		{#each contextsStore.items as ctx (ctx.id)}
			{@const ctxHref = resolve('/(app)/context/[id]', { id: String(ctx.id) })}
			{@const ctxActive = isActive(ctxHref)}
			<div class="group flex items-center gap-1 pr-1">
				<a
					href={ctxHref}
					class="flex flex-1 items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors hover:bg-sidebar-accent"
					class:bg-sidebar-accent={ctxActive}
					class:font-medium={ctxActive}
				>
					<span
						class="inline-block size-2 shrink-0 rounded-full"
						style={`background-color: ${ctx.color}`}
					></span>
					<span class="truncate">{ctx.name}</span>
				</a>
				<button
					type="button"
					class="rounded-md p-1 opacity-0 transition-opacity group-hover:opacity-100 hover:bg-accent hover:text-foreground"
					onclick={() => {
						projectDialogContextId = ctx.id;
						projectDialogOpen = true;
					}}
					aria-label={`Add project to ${ctx.name}`}
					title="Add project"
				>
					<PlusIcon class="size-3.5" />
				</button>
			</div>
			{#each projectsStore.byContext(ctx.id) as project (project.id)}
				{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
				{@const active = isActive(href)}
				<a
					{href}
					class="flex items-center gap-2 rounded-md py-1.5 pl-7 pr-2.5 text-sm transition-colors hover:bg-sidebar-accent"
					class:bg-sidebar-accent={active}
					class:font-medium={active}
				>
					<FolderIcon class="size-3.5 shrink-0 text-muted-foreground" />
					<span class="truncate">{project.title}</span>
				</a>
			{/each}
		{/each}
	</SidebarSection>

	<SidebarSection title="Labels" collapsible onAdd={() => (labelDialogOpen = true)}>
		{#each labelsOrdered as label (label.id)}
			{@const href = resolve('/(app)/label/[id]', { id: String(label.id) })}
			{@const active = isActive(href)}
			<a
				{href}
				class="flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors hover:bg-sidebar-accent"
				class:bg-sidebar-accent={active}
				class:font-medium={active}
			>
				<TagIcon class="size-3.5 shrink-0" style={`color: ${label.color}`} weight="fill" />
				<span class="truncate">{label.name}</span>
			</a>
		{/each}
	</SidebarSection>
</aside>

<ContextDialog bind:open={contextDialogOpen} />
<LabelDialog bind:open={labelDialogOpen} />
<ProjectDialog bind:open={projectDialogOpen} defaultContextId={projectDialogContextId} />
