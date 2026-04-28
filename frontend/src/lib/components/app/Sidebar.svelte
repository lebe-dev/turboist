<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
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
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import SidebarSection from './SidebarSection.svelte';

	const weekLimit = $derived(configStore.value?.weekly.limit);
	const backlogLimit = $derived(configStore.value?.backlog.limit);

	const navItems = $derived([
		{ href: resolve('/inbox'), label: 'Inbox', icon: InboxIcon },
		{ href: resolve('/today'), label: 'Today', icon: SunIcon },
		{ href: resolve('/tomorrow'), label: 'Tomorrow', icon: SunHorizonIcon },
		{ href: resolve('/week'), label: 'Week', icon: CalendarIcon, badge: weekLimit },
		{ href: resolve('/backlog'), label: 'Backlog', icon: StackIcon, badge: backlogLimit },
		{ href: resolve('/overdue'), label: 'Overdue', icon: WarningIcon },
		{ href: resolve('/search'), label: 'Search', icon: MagnifyingGlassIcon }
	]);

	function isActive(href: string): boolean {
		return page.url.pathname === href;
	}

	const labelsOrdered = $derived([...labelsStore.favourites, ...labelsStore.rest]);
</script>

<aside
	class="flex h-full w-64 shrink-0 flex-col overflow-y-auto border-r border-sidebar-border bg-sidebar text-sidebar-foreground"
>
	<div class="px-4 py-3 text-sm font-semibold tracking-wide">Turboist</div>

	<nav class="flex flex-col gap-px px-2 py-1" aria-label="Main">
		{#each navItems as item (item.href)}
			{@const Icon = item.icon}
			<a
				href={item.href}
				class="flex items-center justify-between rounded px-2 py-1.5 text-sm hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
				class:bg-sidebar-accent={isActive(item.href)}
				class:text-sidebar-accent-foreground={isActive(item.href)}
				aria-current={isActive(item.href) ? 'page' : undefined}
			>
				<span class="flex items-center gap-2">
					<Icon class="size-4" />
					{item.label}
				</span>
				{#if item.badge != null}
					<span class="text-xs text-muted-foreground">{item.badge}</span>
				{/if}
			</a>
		{/each}
	</nav>

	{#if projectsStore.pinned.length > 0}
		<SidebarSection title="Pinned">
			{#each projectsStore.pinned as project (project.id)}
				{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
				<a
					{href}
					class="flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-sidebar-accent"
					class:bg-sidebar-accent={isActive(href)}
				>
					<PushPinIcon class="size-3" />
					<span class="truncate">{project.title}</span>
				</a>
			{/each}
		</SidebarSection>
	{/if}

	<SidebarSection title="Contexts" collapsible>
		{#each contextsStore.items as ctx (ctx.id)}
			{@const ctxHref = resolve('/(app)/context/[id]', { id: String(ctx.id) })}
			<a
				href={ctxHref}
				class="flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-sidebar-accent"
				class:bg-sidebar-accent={isActive(ctxHref)}
			>
				<span
					class="inline-block size-2 shrink-0 rounded-full"
					style={`background-color: ${ctx.color}`}
				></span>
				<span class="truncate">{ctx.name}</span>
			</a>
			{#each projectsStore.byContext(ctx.id) as project (project.id)}
				{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
				<a
					{href}
					class="flex items-center gap-2 rounded pl-6 pr-2 py-1 text-sm hover:bg-sidebar-accent"
					class:bg-sidebar-accent={isActive(href)}
				>
					<FolderIcon class="size-3" />
					<span class="truncate">{project.title}</span>
				</a>
			{/each}
		{/each}
	</SidebarSection>

	{#if labelsOrdered.length > 0}
		<SidebarSection title="Labels" collapsible>
			{#each labelsOrdered as label (label.id)}
				{@const href = resolve('/(app)/label/[id]', { id: String(label.id) })}
				<a
					{href}
					class="flex items-center gap-2 rounded px-2 py-1 text-sm hover:bg-sidebar-accent"
					class:bg-sidebar-accent={isActive(href)}
				>
					<TagIcon class="size-3" style={`color: ${label.color}`} />
					<span class="truncate">{label.name}</span>
				</a>
			{/each}
		</SidebarSection>
	{/if}
</aside>
