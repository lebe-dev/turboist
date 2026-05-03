<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import LightningIcon from 'phosphor-svelte/lib/Lightning';
	import InboxIcon from 'phosphor-svelte/lib/Tray';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
	import CalendarIcon from 'phosphor-svelte/lib/Calendar';
	import StackIcon from 'phosphor-svelte/lib/Stack';
	import CalendarCheckIcon from 'phosphor-svelte/lib/CalendarCheck';
	import CheckCircleIcon from 'phosphor-svelte/lib/CheckCircle';
	import MagnifyingGlassIcon from 'phosphor-svelte/lib/MagnifyingGlass';
	import PushPinIcon from 'phosphor-svelte/lib/PushPin';
	import FolderIcon from 'phosphor-svelte/lib/Folder';
	import TagIcon from 'phosphor-svelte/lib/Tag';
	import SidebarSimpleIcon from 'phosphor-svelte/lib/SidebarSimple';
	import SignOutIcon from 'phosphor-svelte/lib/SignOut';
	import GearIcon from 'phosphor-svelte/lib/Gear';
	import UserIcon from 'phosphor-svelte/lib/User';
	import XIcon from 'phosphor-svelte/lib/X';
	import { toast } from 'svelte-sonner';
	import { tasks as tasksApi } from '$lib/api/endpoints/tasks';
	import { projects as projectsApi } from '$lib/api/endpoints/projects';
	import { getApiClient } from '$lib/api/client';
	import { describeError } from '$lib/utils/taskActions';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';
	import { planStatsStore } from '$lib/stores/planStats.svelte';
	import { inboxStatsStore } from '$lib/stores/inboxStats.svelte';
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
		danger?: boolean;
	};

	const inboxOverflow = $derived(inboxStatsStore.warnThresholdExceeded);

	const primaryNav = $derived<NavItem[]>([
		{
			href: resolve('/inbox'),
			label: 'Inbox',
			icon: InboxIcon,
			accent: !inboxOverflow,
			danger: inboxOverflow,
			current: inboxOverflow ? inboxStatsStore.count : undefined
		},
		{ href: resolve('/today'), label: 'Today', icon: SunIcon },
		{ href: resolve('/tomorrow'), label: 'Tomorrow', icon: SunHorizonIcon },
		{
			href: resolve('/week'),
			label: 'Week',
			icon: CalendarIcon,
			current: planStatsStore.value?.week,
			limit: weekLimit
		},
		{ href: resolve('/completed'), label: 'Completed', icon: CheckCircleIcon }
	]);

	const planningNav = $derived<NavItem[]>([
		{
			href: resolve('/backlog'),
			label: 'Backlog',
			icon: StackIcon,
			current: planStatsStore.value?.backlog,
			limit: backlogLimit
		},
		{
			href: resolve('/next-week'),
			label: 'Next week',
			icon: CalendarCheckIcon,
			current: planStatsStore.value?.week,
			limit: weekLimit
		},
		{ href: resolve('/search'), label: 'Search', icon: MagnifyingGlassIcon }
	]);

	const filteredProjects = $derived.by(() => {
		const active = userStateStore.activeContextId;
		const all = projectsStore.items ?? [];
		const scoped = active == null ? all : all.filter((p) => p.contextId === active);
		return [...scoped].sort((a, b) => a.title.localeCompare(b.title));
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
		inboxStatsStore.clear();
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

	async function unpinProject(id: number): Promise<void> {
		try {
			const updated = await projectsApi.unpin(getApiClient(), id);
			projectsStore.upsert(updated);
		} catch (err) {
			toast.error(describeError(err, 'Failed to unpin'));
		}
	}

	async function unpinTask(id: number): Promise<void> {
		try {
			await tasksApi.unpin(getApiClient(), id);
			pinnedTasksStore.removeItem(id);
		} catch (err) {
			toast.error(describeError(err, 'Failed to unpin'));
		}
	}
</script>

{#snippet unpinButton(action: () => void, label: string)}
	<button
		type="button"
		class="mr-1 flex size-5 shrink-0 items-center justify-center rounded text-muted-foreground opacity-0 transition-all hover:bg-sidebar-border hover:text-foreground focus:opacity-100 group-hover/pin:opacity-100"
		onclick={action}
		aria-label={label}
		title={label}
	>
		<XIcon class="size-3" weight="bold" />
	</button>
{/snippet}

{#snippet navLink(item: NavItem)}
	{@const Icon = item.icon}
	{@const active = isActive(item.href)}
	{@const showBadge = item.current != null || item.limit != null}
	<a
		href={item.href as ReturnType<typeof resolve>}
		class="group/nav flex items-center justify-between gap-2 rounded-md px-2.5 py-1 text-[13px] text-muted-foreground transition-colors"
		class:bg-sidebar-accent={active}
		class:text-foreground={active && !item.accent && !item.danger}
		class:text-primary={item.accent && !item.danger}
		class:text-red-600={item.danger}
		class:dark:text-red-400={item.danger}
		class:hover:bg-sidebar-accent={!active}
		class:hover:text-foreground={!active && !item.accent && !item.danger}
		aria-current={active ? 'page' : undefined}
	>
		<span class="flex min-w-0 items-center gap-2.5">
			<Icon
				class={item.danger ? 'size-[16px] shrink-0' : 'size-[16px] shrink-0 opacity-80'}
				weight={active || item.danger ? 'fill' : 'regular'}
			/>
			<span class="truncate">{item.label}</span>
		</span>
		{#if showBadge}
			<span
				class="font-mono text-[10px] tabular-nums"
				class:text-muted-foreground={!item.danger}
				class:text-red-600={item.danger}
				class:dark:text-red-400={item.danger}
				class:font-semibold={item.danger}
			>
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
				class="flex size-5 items-center justify-center rounded-sm bg-primary text-primary-foreground shadow-sm"
			>
				<LightningIcon class="size-3" weight="fill" />
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
					<div
						class="group/pin relative flex items-start rounded-md transition-colors hover:bg-sidebar-accent"
						class:bg-sidebar-accent={active}
					>
						<a
							{href}
							class="flex min-w-0 flex-1 items-start gap-2.5 px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:text-foreground"
							class:text-foreground={active}
						>
							<PushPinIcon class="mt-0.5 size-3.5 shrink-0 text-amber-500/80" weight="fill" />
							<span class="break-words">{project.title}</span>
						</a>
						{@render unpinButton(() => unpinProject(project.id), `Unpin ${project.title}`)}
					</div>
				{/each}
				{#each pinnedTasksStore.items as task (`t-${task.id}`)}
					{@const href = resolve('/(app)/task/[id]', { id: String(task.id) })}
					{@const active = isActive(href)}
					<div
						class="group/pin relative flex items-start rounded-md transition-colors hover:bg-sidebar-accent"
						class:bg-sidebar-accent={active}
					>
						<a
							{href}
							class="flex min-w-0 flex-1 items-start gap-2.5 px-2.5 py-1 text-[13px] text-muted-foreground transition-colors hover:text-foreground"
							class:text-foreground={active}
						>
							<PushPinIcon class="mt-0.5 size-3.5 shrink-0 text-amber-500/80" weight="regular" />
							<span class="break-words">{task.title}</span>
						</a>
						{@render unpinButton(() => unpinTask(task.id), `Unpin ${task.title}`)}
					</div>
				{/each}
			</SidebarSection>
		{/if}

		<SidebarSection
			title="Projects"
			collapsible
			onAdd={() => {
				if (contextsStore.items.length === 0) {
					toast.error('Create a context first');
					return;
				}
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
					<FolderIcon
						class="size-3.5 shrink-0 opacity-90"
						style={`color: ${project.color}`}
						weight="fill"
					/>
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
				<DropdownMenu.Item onclick={() => goto(resolve('/settings'))}>
					<GearIcon class="size-4" />
					Settings
				</DropdownMenu.Item>
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
