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
	import SidebarSimpleIcon from 'phosphor-svelte/lib/SidebarSimple';
	import SignOutIcon from 'phosphor-svelte/lib/SignOut';
	import UserIcon from 'phosphor-svelte/lib/User';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { planStatsStore } from '$lib/stores/planStats.svelte';
	import { pinnedTasksStore } from '$lib/stores/pinnedTasks.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';
	import { sidebarStore } from '$lib/stores/sidebar.svelte';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { goto } from '$app/navigation';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import SidebarSection from './SidebarSection.svelte';
	import LabelDialog from '$lib/components/dialog/LabelDialog.svelte';
	import ProjectDialog from '$lib/components/dialog/ProjectDialog.svelte';

	let labelDialogOpen = $state(false);
	let projectDialogOpen = $state(false);
	let projectDialogContextId = $state<number | null>(null);

	const auth = getAuthStore();
	const appVersion = __APP_VERSION__;

	const weekLimit = $derived(configStore.value?.weekly.limit);
	const backlogLimit = $derived(configStore.value?.backlog.limit);

	type NavItem = {
		href: string;
		label: string;
		icon: typeof InboxIcon;
		current?: number;
		limit?: number;
		accent?: boolean;
	};

	const primaryNav = $derived<NavItem[]>([
		{ href: resolve('/inbox'), label: 'Inbox', icon: InboxIcon, accent: true },
		{ href: resolve('/today'), label: 'Today', icon: SunIcon },
		{ href: resolve('/tomorrow'), label: 'Tomorrow', icon: SunHorizonIcon },
		{
			href: resolve('/week'),
			label: 'Week',
			icon: CalendarIcon,
			current: planStatsStore.value?.week,
			limit: weekLimit
		}
	]);

	const planningNav = $derived<NavItem[]>([
		{
			href: resolve('/backlog'),
			label: 'Backlog',
			icon: StackIcon,
			current: planStatsStore.value?.backlog,
			limit: backlogLimit
		},
		{ href: resolve('/overdue'), label: 'Overdue', icon: WarningIcon },
		{ href: resolve('/search'), label: 'Search', icon: MagnifyingGlassIcon }
	]);

	const filteredProjects = $derived.by(() => {
		const active = userStateStore.activeContextId;
		const all = projectsStore.items ?? [];
		if (active == null) return all;
		return all.filter((p) => p.contextId === active);
	});

	function isActive(href: string): boolean {
		return page.url.pathname === href;
	}

	const labelsOrdered = $derived([...labelsStore.favourites, ...labelsStore.rest]);

	function clearStores(): void {
		contextsStore.clear();
		projectsStore.clear();
		labelsStore.clear();
		configStore.clear();
		planStatsStore.clear();
		pinnedTasksStore.clear();
		userStateStore.clear();
	}

	async function onLogout(): Promise<void> {
		await auth.logout();
		clearStores();
		await goto(resolve('/login'));
	}

	async function onLogoutAll(): Promise<void> {
		await auth.logoutAll();
		clearStores();
		await goto(resolve('/login'));
	}
</script>

{#snippet navLink(item: NavItem)}
	{@const Icon = item.icon}
	{@const active = isActive(item.href)}
	{@const showBadge = item.current != null || item.limit != null}
	<a
		href={item.href as ReturnType<typeof resolve>}
		class="group/nav flex items-center justify-between gap-2 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors"
		class:bg-sidebar-accent={active}
		class:text-foreground={active && !item.accent}
		class:text-primary={item.accent}
		class:hover:bg-sidebar-accent={!active}
		class:hover:text-foreground={!active && !item.accent}
		aria-current={active ? 'page' : undefined}
	>
		<span class="flex min-w-0 items-center gap-2.5">
			<Icon class="size-[16px] shrink-0 opacity-80" weight={active ? 'fill' : 'regular'} />
			<span class="truncate">{item.label}</span>
		</span>
		{#if showBadge}
			<span class="font-mono text-[10px] tabular-nums text-muted-foreground/70">
				{#if item.current != null && item.limit != null}
					{item.current}/{item.limit}
				{:else if item.limit != null}
					{item.limit}
				{:else}
					{item.current}
				{/if}
			</span>
		{/if}
	</a>
{/snippet}

<aside
	class="flex h-full w-64 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground"
>
	<div class="flex items-center justify-between gap-2 px-4 pb-3 pt-4">
		<div class="flex min-w-0 items-center gap-2">
			<span
				class="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground shadow-sm"
			>
				<LightningIcon class="size-4" weight="fill" />
			</span>
			<span class="text-[13px] font-semibold uppercase tracking-[0.18em]">Turboist</span>
		</div>
		<button
			type="button"
			class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
			onclick={() => sidebarStore.toggle()}
			aria-label="Collapse sidebar"
			title="Collapse sidebar"
		>
			<SidebarSimpleIcon class="size-4" />
		</button>
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
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

		{#if projectsStore.pinned.length > 0 || pinnedTasksStore.items.length > 0}
			<SidebarSection title="Pinned">
				{#each projectsStore.pinned as project (`p-${project.id}`)}
					{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
					{@const active = isActive(href)}
					<a
						{href}
						class="flex items-center gap-2.5 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
						class:bg-sidebar-accent={active}
						class:text-foreground={active}
					>
						<PushPinIcon class="size-3.5 shrink-0 text-amber-500/80" weight="fill" />
						<span class="truncate">{project.title}</span>
					</a>
				{/each}
				{#each pinnedTasksStore.items as task (`t-${task.id}`)}
					{@const href = resolve('/(app)/task/[id]', { id: String(task.id) })}
					{@const active = isActive(href)}
					<a
						{href}
						class="flex items-center gap-2.5 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
						class:bg-sidebar-accent={active}
						class:text-foreground={active}
					>
						<PushPinIcon class="size-3.5 shrink-0 text-amber-500/80" weight="regular" />
						<span class="truncate">{task.title}</span>
					</a>
				{/each}
			</SidebarSection>
		{/if}

		<SidebarSection
			title="Projects"
			collapsible
			onAdd={() => {
				projectDialogContextId = userStateStore.activeContextId;
				projectDialogOpen = true;
			}}
		>
			{#each filteredProjects as project (project.id)}
				{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
				{@const active = isActive(href)}
				<a
					{href}
					class="flex items-center gap-2.5 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
					class:bg-sidebar-accent={active}
					class:text-foreground={active}
				>
					<FolderIcon class="size-3.5 shrink-0 opacity-70" />
					<span class="truncate">{project.title}</span>
				</a>
			{/each}
		</SidebarSection>

		<SidebarSection title="Labels" collapsible onAdd={() => (labelDialogOpen = true)}>
			{#each labelsOrdered as label (label.id)}
				{@const href = resolve('/(app)/label/[id]', { id: String(label.id) })}
				{@const active = isActive(href)}
				<a
					{href}
					class="flex items-center gap-2.5 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
					class:bg-sidebar-accent={active}
					class:text-foreground={active}
				>
					<TagIcon class="size-3.5 shrink-0 opacity-90" style={`color: ${label.color}`} weight="fill" />
					<span class="truncate">{label.name}</span>
				</a>
			{/each}
		</SidebarSection>
	</div>

	<div class="mt-auto flex flex-col gap-0.5 border-t border-sidebar-border px-2 py-2">
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button
						{...props}
						type="button"
						class="flex items-center gap-2.5 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
					>
						<UserIcon class="size-[16px] shrink-0 opacity-80" />
						<span class="truncate">{auth.user?.username ?? ''}</span>
					</button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content align="start" side="top" class="w-48">
				<DropdownMenu.Label>{auth.user?.username ?? ''}</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={onLogoutAll}>Log out everywhere</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
		<button
			type="button"
			onclick={onLogout}
			class="flex items-center justify-between gap-2 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground"
		>
			<span class="flex min-w-0 items-center gap-2.5">
				<SignOutIcon class="size-[16px] shrink-0 opacity-80" />
				<span class="truncate">Log out</span>
			</span>
			<span class="font-mono text-[10px] tabular-nums text-muted-foreground/70">v{appVersion}</span>
		</button>
	</div>
</aside>

<LabelDialog bind:open={labelDialogOpen} />
<ProjectDialog bind:open={projectDialogOpen} defaultContextId={projectDialogContextId} />
