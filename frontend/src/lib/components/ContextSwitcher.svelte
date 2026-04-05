<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { pinnedStore } from '$lib/stores/pinned.svelte';
	import { planningStore } from '$lib/stores/planning.svelte';
	import { appStore } from '$lib/stores/app.svelte';
	import { tasksStore } from '$lib/stores/tasks.svelte';
	import TagIcon from '@lucide/svelte/icons/tag';
	import LightbulbIcon from '@lucide/svelte/icons/lightbulb';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import ListIcon from '@lucide/svelte/icons/list';
	import CalendarDaysIcon from '@lucide/svelte/icons/calendar-days';
	import ArchiveIcon from '@lucide/svelte/icons/archive';
	import CalendarRangeIcon from '@lucide/svelte/icons/calendar-range';
	import SunIcon from '@lucide/svelte/icons/sun';
	import SunriseIcon from '@lucide/svelte/icons/sunrise';
	import CircleCheckBigIcon from '@lucide/svelte/icons/circle-check-big';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import PinIcon from '@lucide/svelte/icons/pin';
	import XIcon from '@lucide/svelte/icons/x';
	import { t } from 'svelte-intl-precompile';

	const INBOX_ALERT_THRESHOLD = 5;

	let { collapsed = false, onItemClick }: { collapsed?: boolean; onItemClick?: () => void } = $props();

	function navigateToMainIfNeeded() {
		if ($page.url.pathname !== '/') {
			goto('/');
		}
	}

	const viewDefs = [
		{ id: 'inbox' as const, key: 'views.inbox', icon: InboxIcon },
		{ id: 'today' as const, key: 'views.today', icon: SunIcon },
		{ id: 'tomorrow' as const, key: 'views.tomorrow', icon: SunriseIcon },
		{ id: 'weekly' as const, key: 'views.weekly', icon: CalendarDaysIcon },
		{ id: 'all' as const, key: 'views.all', icon: ListIcon },
		{ id: 'completed' as const, key: 'views.completed', icon: CircleCheckBigIcon }
	];

	function unpinTask(e: MouseEvent, taskId: string) {
		e.stopPropagation();
		pinnedStore.unpin(taskId);
	}
</script>

<nav class="flex flex-col gap-0.5">
	{#each viewDefs as view (view.id)}
		{@const ViewIcon = view.icon}
		{@const viewLabel = $t(view.key)}
		{@const isInboxAlert = view.id === 'inbox' && tasksStore.inboxCount > INBOX_ALERT_THRESHOLD}
		<button
			class="group flex cursor-pointer items-center rounded-lg text-[15px] md:text-[13px] transition-all duration-150
				{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2 md:py-1.5'}
				{planningStore.active
				? 'text-sidebar-foreground/40'
				: contextsStore.activeView === view.id
					? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
					: isInboxAlert
						? 'text-red-500 hover:bg-sidebar-accent/50 hover:text-red-400'
						: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
			onclick={() => { if (planningStore.active) planningStore.exit(); navigateToMainIfNeeded(); contextsStore.setView(view.id); onItemClick?.(); }}
			title={collapsed ? viewLabel : undefined}
		>
			<ViewIcon class="h-4 w-4 md:h-3.5 md:w-3.5 shrink-0 {isInboxAlert ? 'opacity-80' : 'opacity-60'}" />
			{#if !collapsed}
				{viewLabel}
				{#if isInboxAlert}
					<span class="ml-auto text-[11px] font-semibold tabular-nums">{tasksStore.inboxCount}</span>
				{/if}
			{/if}
		</button>
	{/each}

	<!-- Pinned Tasks -->
	{#if pinnedStore.items.length > 0}
		<div class="my-3 border-t border-sidebar-border"></div>

		{#if !collapsed}
			<p class="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
				{$t('sidebar.pinned')}{#if pinnedStore.isFull} ({pinnedStore.items.length}/{pinnedStore.maxPinned}){/if}
			</p>
		{/if}

		{#each pinnedStore.items as pinned (pinned.id)}
			<a
				href="/task/{pinned.id}"
				class="group flex cursor-pointer items-center rounded-lg text-[15px] md:text-[13px] transition-all duration-150
					{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2 md:py-1.5'}
					text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground"
				title={collapsed ? pinned.content : undefined}
			onclick={() => onItemClick?.()}
			>
				<PinIcon class="h-4 w-4 md:h-3.5 md:w-3.5 shrink-0 opacity-60" />
				{#if !collapsed}
					<span class="flex-1 break-words text-left text-[11px] leading-tight">{pinned.content}</span>
					<span
						class="flex h-4 w-4 shrink-0 items-center justify-center rounded text-muted-foreground/40 opacity-0 transition-opacity group-hover:opacity-100 hover:text-foreground"
						role="button"
						tabindex="-1"
						onclick={(e: MouseEvent) => unpinTask(e, pinned.id)}
						onkeydown={() => {}}
					>
						<XIcon class="h-3 w-3" />
					</span>
				{/if}
			</a>
		{/each}
	{/if}

	<!-- Planning section -->
	<div class="my-3 border-t border-sidebar-border"></div>

	{#if !collapsed}
		<p class="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
			{$t('sidebar.planning')}
		</p>
	{/if}

	<button
		class="group flex w-full cursor-pointer items-center rounded-lg text-[15px] md:text-[13px] transition-all duration-150
			{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2 md:py-1.5'}
			{!planningStore.active && contextsStore.activeView === 'backlog'
			? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
			: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
		onclick={() => { if (planningStore.active) planningStore.exit(); navigateToMainIfNeeded(); contextsStore.setView('backlog'); onItemClick?.(); }}
		title={collapsed ? $t('views.backlog') : undefined}
	>
		<ArchiveIcon class="h-4 w-4 md:h-3.5 md:w-3.5 shrink-0 opacity-60" />
		{#if !collapsed}
			{$t('views.backlog')}
		{/if}
	</button>

	<button
		class="group flex w-full cursor-pointer items-center rounded-lg text-[15px] md:text-[13px] transition-all duration-150
			{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2 md:py-1.5'}
			{planningStore.active
			? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
			: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
		onclick={() => { navigateToMainIfNeeded(); if (planningStore.active) { planningStore.exit(); } else { planningStore.enter(); } onItemClick?.(); }}
		title={collapsed ? $t('planning.title') : undefined}
	>
		<CalendarRangeIcon class="h-4 w-4 md:h-3.5 md:w-3.5 shrink-0 opacity-60" />
		{#if !collapsed}
			{$t('planning.title')}
		{/if}
	</button>

	<!-- Labels -->
	<a
		href="/labels"
		class="group flex cursor-pointer items-center rounded-lg text-[15px] md:text-[13px] transition-all duration-150
			{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2 md:py-1.5'}
			{$page.url.pathname === ('/labels' as string)
			? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
			: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
		title={collapsed ? $t('sidebar.labels') : undefined}
		onclick={() => onItemClick?.()}
	>
		<TagIcon class="h-4 w-4 md:h-3.5 md:w-3.5 shrink-0 opacity-60" />
		{#if !collapsed}
			{$t('sidebar.labels')}
		{/if}
	</a>

	<!-- Ideas -->
	<div class="my-3 border-t border-sidebar-border"></div>

	<button
		class="group flex w-full cursor-pointer items-center rounded-lg text-[15px] md:text-[13px] transition-all duration-150
			{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2 md:py-1.5'}
			text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-yellow-400"
		onclick={() => { appStore.quickCaptureOpen = true; onItemClick?.(); }}
		title={collapsed ? $t('quickCapture.title') : undefined}
	>
		<LightbulbIcon class="h-4 w-4 md:h-3.5 md:w-3.5 shrink-0 opacity-60" />
		{#if !collapsed}
			{$t('quickCapture.title')}
		{/if}
	</button>

	<!-- Projects -->
	{#if appStore.projects.length > 0}
		<div class="my-3 border-t border-sidebar-border"></div>

		{#if !collapsed}
			<p class="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
				{$t('sidebar.projects')}
			</p>
		{/if}

		{#each appStore.projects.filter((p) => p.id !== appStore.inboxProjectId) as project (project.id)}
			<a
				href="/projects/{project.id}"
				class="group flex cursor-pointer items-center rounded-lg text-[15px] md:text-[13px] transition-all duration-150
					{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2 md:py-1.5'}
					{$page.url.pathname === `/projects/${project.id}`
					? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
					: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
				title={collapsed ? project.name : undefined}
				onclick={() => onItemClick?.()}
			>
				<FolderIcon class="h-4 w-4 md:h-3.5 md:w-3.5 shrink-0 opacity-60" />
				{#if !collapsed}
					<span class="truncate">{project.name}</span>
				{/if}
			</a>
		{/each}
	{/if}

</nav>
