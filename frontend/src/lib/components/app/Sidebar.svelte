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
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { isLabelVisible, isProjectVisible } from '$lib/utils/visibility';
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { goto } from '$app/navigation';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import SidebarSection from './SidebarSection.svelte';
	import LabelDialog from '$lib/components/dialog/LabelDialog.svelte';
	import ProjectDialog from '$lib/components/dialog/ProjectDialog.svelte';
	import TroikiTriggerIcon from './TroikiTriggerIcon.svelte';
	import type { TroikiCategory } from '$lib/api/types';
	import { t } from '$lib/i18n';

	let labelDialogOpen = $state(false);
	let projectDialogOpen = $state(false);
	let projectDialogContextId = $state<number | null>(null);

	const auth = getAuthStore();

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

	const inboxWarnThreshold = $derived(configStore.value?.inbox.warnThreshold ?? 0);
	const inboxOverflow = $derived(inboxWarnThreshold > 0 && inboxStatsStore.count > inboxWarnThreshold);

	const primaryNav = $derived<NavItem[]>([
		{
			href: resolve('/inbox'),
			label: $t('nav.inbox'),
			icon: InboxIcon,
			accent: !inboxOverflow,
			danger: inboxOverflow,
			current: inboxOverflow ? inboxStatsStore.count : undefined
		},
		{ href: resolve('/today'), label: $t('nav.today'), icon: SunIcon },
		{ href: resolve('/tomorrow'), label: $t('nav.tomorrow'), icon: SunHorizonIcon },
		{
			href: resolve('/week'),
			label: $t('nav.week'),
			icon: CalendarIcon,
			current: planStatsStore.value?.week,
			limit: weekLimit
		},
		{ href: resolve('/completed'), label: $t('nav.completed'), icon: CheckCircleIcon }
	]);

	const planningNav = $derived<NavItem[]>([
		{
			href: resolve('/backlog'),
			label: $t('nav.backlog'),
			icon: StackIcon,
			current: planStatsStore.value?.backlog,
			limit: backlogLimit
		},
		{
			href: resolve('/next-week'),
			label: $t('nav.nextWeek'),
			icon: CalendarCheckIcon,
			current: planStatsStore.value?.week,
			limit: weekLimit
		},
		{ href: resolve('/search'), label: $t('nav.search'), icon: MagnifyingGlassIcon }
	]);

	const TROIKI_ORDER: Record<TroikiCategory, number> = { important: 0, medium: 1, rest: 2 };

	const filteredProjects = $derived.by(() => {
		const active = userStateStore.activeContextId;
		const all = projectsStore.items ?? [];
		const scoped = active == null ? all : all.filter((p) => p.contextId === active);
		const visible = scoped.filter((p) => isProjectVisible(p, settingsStore.publicView));
		return [...visible].sort((a, b) => {
			const ta = a.troikiCategory;
			const tb = b.troikiCategory;
			if (ta && !tb) return -1;
			if (!ta && tb) return 1;
			if (ta && tb && ta !== tb) return TROIKI_ORDER[ta] - TROIKI_ORDER[tb];
			return a.title.localeCompare(b.title);
		});
	});

	function isActive(href: string): boolean {
		return page.url.pathname === href;
	}

	const labelsOrdered = $derived(
		[...labelsStore.favourites, ...labelsStore.rest].filter((l) =>
			isLabelVisible(l, settingsStore.publicView)
		)
	);

	const sidebarPinnedProjects = $derived(
		projectsStore.pinned.filter((p) => isProjectVisible(p, settingsStore.publicView))
	);
	const sidebarPinnedTasks = $derived(
		pinnedTasksStore.items.filter((task) => {
			if (!settingsStore.publicView) return true;
			if (task.isPrivate) return false;
			if (task.projectId !== null) {
				const project = projectsStore.items.find((p) => p.id === task.projectId);
				if (project && project.isPrivate) return false;
			}
			return true;
		})
	);

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
			toast.error(describeError(err, $t('sidebar.failedUnpin')));
		}
	}

	async function unpinTask(id: number): Promise<void> {
		try {
			await tasksApi.unpin(getApiClient(), id);
			pinnedTasksStore.removeItem(id);
		} catch (err) {
			toast.error(describeError(err, $t('sidebar.failedUnpin')));
		}
	}
</script>

{#snippet unpinButton(action: () => void, label: string)}
	<button
		type="button"
		class="mr-1 flex size-5 shrink-0 self-center items-center justify-center rounded text-muted-foreground opacity-0 transition-all hover:bg-sidebar-border hover:text-foreground focus:opacity-100 group-hover/pin:opacity-100"
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
		class="group/nav flex items-center justify-between gap-2 rounded-md px-2.5 py-2.5 text-[15px] text-muted-foreground transition-colors md:py-1 md:text-[13px]"
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
				class={item.danger ? 'size-[18px] shrink-0 md:size-[16px]' : 'size-[18px] shrink-0 opacity-80 md:size-[16px]'}
				weight={active || item.danger ? 'fill' : 'regular'}
			/>
			<span class="truncate">{item.label}</span>
		</span>
		{#if showBadge}
			<span
				class="font-mono text-[12px] tabular-nums md:text-[10px]"
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
			aria-label={$t('sidebar.collapse')}
			title={$t('sidebar.collapse')}
		>
			<SidebarSimpleIcon class="size-4" />
		</button>
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
		<nav class="flex flex-col gap-0.5 px-2 pb-2" aria-label={$t('nav.main')}>
			{#each primaryNav as item (item.href)}
				{@render navLink(item)}
			{/each}
		</nav>

		<SidebarSection title={$t('nav.planning')}>
			{#each planningNav as item (item.href)}
				{@render navLink(item)}
			{/each}
		</SidebarSection>

		{#if sidebarPinnedProjects.length > 0 || sidebarPinnedTasks.length > 0}
			<SidebarSection title={$t('nav.pinned')}>
				{#each sidebarPinnedProjects as project (`p-${project.id}`)}
					{@const href = resolve('/(app)/project/[id]', { id: String(project.id) })}
					{@const active = isActive(href)}
					<div
						class="group/pin relative flex items-start rounded-md transition-colors hover:bg-sidebar-accent"
						class:bg-sidebar-accent={active}
					>
						<a
							{href}
							class="flex min-w-0 flex-1 items-start gap-2.5 px-2.5 py-2.5 text-[15px] text-muted-foreground transition-colors hover:text-foreground md:py-1 md:text-[13px]"
							class:text-foreground={active}
						>
							<PushPinIcon class="mt-0.5 size-4 shrink-0 text-amber-500/80 md:size-3.5" weight="fill" />
							<span class="break-words">{project.title}</span>
						</a>
						{@render unpinButton(() => unpinProject(project.id), $t('sidebar.unpinAria', { values: { name: project.title } }))}
					</div>
				{/each}
				{#each sidebarPinnedTasks as task (`t-${task.id}`)}
					{@const href = resolve('/(app)/task/[id]', { id: String(task.id) })}
					{@const active = isActive(href)}
					<div
						class="group/pin relative flex items-start rounded-md transition-colors hover:bg-sidebar-accent"
						class:bg-sidebar-accent={active}
					>
						<a
							{href}
							class="flex min-w-0 flex-1 items-start gap-2.5 px-2.5 py-2.5 text-[15px] text-muted-foreground transition-colors hover:text-foreground md:py-1 md:text-[13px]"
							class:text-foreground={active}
						>
							<PushPinIcon class="mt-0.5 size-4 shrink-0 text-amber-500/80 md:size-3.5" weight="regular" />
							<span class="break-words">{task.title}</span>
						</a>
						{@render unpinButton(() => unpinTask(task.id), $t('sidebar.unpinAria', { values: { name: task.title } }))}
					</div>
				{/each}
			</SidebarSection>
		{/if}

		<SidebarSection
			title={$t('nav.projects')}
			collapsible
			storageKey="sidebar:projects:open"
			onAdd={() => {
				if (contextsStore.items.length === 0) {
					toast.error($t('sidebar.createContextFirst'));
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
					class="flex items-start gap-2.5 rounded-md px-2.5 py-2.5 text-[15px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground md:py-1 md:text-[13px]"
					class:bg-sidebar-accent={active}
					class:text-foreground={active}
				>
					<FolderIcon
						class="mt-0.5 size-4 shrink-0 opacity-90 md:size-3.5"
						style={`color: ${project.color}`}
						weight="fill"
					/>
					<span class="min-w-0 break-words">
						{project.title}{#if project.troikiCategory}<TroikiTriggerIcon
								class="ml-1.5 inline-block size-3 align-middle text-muted-foreground/50 md:size-2.5"
							/>{/if}{#if project.isPrivate && !settingsStore.publicView}<span
								class="inline-flex align-middle"
								title={$t('common.privateTooltip')}
								aria-label={$t('common.privateMarker')}
							><LockSimpleIcon class="ml-1.5 inline-block size-2.5 text-muted-foreground/40" /></span>{/if}
					</span>
				</a>
			{/each}
		</SidebarSection>

		<SidebarSection title={$t('nav.labels')} collapsible storageKey="sidebar:labels:open" onAdd={() => (labelDialogOpen = true)}>
			{#each labelsOrdered as label (label.id)}
				{@const href = resolve('/(app)/label/[id]', { id: String(label.id) })}
				{@const active = isActive(href)}
				<a
					{href}
					class="flex items-center gap-2.5 rounded-md px-2.5 py-2.5 text-[15px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground md:py-1 md:text-[13px]"
					class:bg-sidebar-accent={active}
					class:text-foreground={active}
				>
					<TagIcon class="size-4 shrink-0 opacity-90 md:size-3.5" style={`color: ${label.color}`} weight="fill" />
					<span class="truncate">{label.name}</span>
					{#if label.isPrivate && !settingsStore.publicView}
						<span
							class="inline-flex shrink-0"
							title={$t('common.privateTooltip')}
							aria-label={$t('common.privateMarker')}
						>
							<LockSimpleIcon class="ml-1 size-2.5 text-muted-foreground/40" />
						</span>
					{/if}
				</a>
			{/each}
		</SidebarSection>
	</div>

	<div class="mt-auto border-t border-sidebar-border px-2 py-2">
		<DropdownMenu.Root>
			<DropdownMenu.Trigger>
				{#snippet child({ props })}
					<button
						{...props}
						type="button"
						class="flex w-full items-center gap-2.5 rounded-md px-2.5 py-2.5 text-[15px] text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-foreground md:py-1 md:text-[13px]"
					>
						<UserIcon class="size-[18px] shrink-0 opacity-80 md:size-[16px]" />
						<span class="truncate">{auth.user?.username ?? ''}</span>
					</button>
				{/snippet}
			</DropdownMenu.Trigger>
			<DropdownMenu.Content align="start" side="top" class="w-48 rounded-md">
				<DropdownMenu.Label>{auth.user?.username ?? ''}</DropdownMenu.Label>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={() => goto(resolve('/settings'))}>
					<GearIcon class="size-4" />
					{$t('nav.settings')}
				</DropdownMenu.Item>
				<DropdownMenu.Separator />
				<DropdownMenu.Item onclick={onLogout}>
					<SignOutIcon class="size-4" />
					{$t('sidebar.logOut')}
				</DropdownMenu.Item>
			</DropdownMenu.Content>
		</DropdownMenu.Root>
	</div>
</aside>

<LabelDialog bind:open={labelDialogOpen} />
<ProjectDialog bind:open={projectDialogOpen} defaultContextId={projectDialogContextId} />
