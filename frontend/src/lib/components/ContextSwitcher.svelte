<script lang="ts">
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { pinnedStore } from '$lib/stores/pinned.svelte';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import ListIcon from '@lucide/svelte/icons/list';
	import CalendarDaysIcon from '@lucide/svelte/icons/calendar-days';
	import CalendarClockIcon from '@lucide/svelte/icons/calendar-clock';
	import SunIcon from '@lucide/svelte/icons/sun';
	import SunriseIcon from '@lucide/svelte/icons/sunrise';
	import CircleCheckBigIcon from '@lucide/svelte/icons/circle-check-big';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import PinIcon from '@lucide/svelte/icons/pin';
	import XIcon from '@lucide/svelte/icons/x';

	let { collapsed = false }: { collapsed?: boolean } = $props();

	const views = [
		{ id: 'inbox' as const, label: 'Входящие', icon: InboxIcon },
		{ id: 'today' as const, label: 'Сегодня', icon: SunIcon },
		{ id: 'tomorrow' as const, label: 'Завтра', icon: SunriseIcon },
		{ id: 'weekly' as const, label: 'На неделе', icon: CalendarDaysIcon },
		{ id: 'next-week' as const, label: 'След. неделю', icon: CalendarClockIcon },
		{ id: 'all' as const, label: 'Все задачи', icon: ListIcon },
		{ id: 'completed' as const, label: 'Выполненные', icon: CircleCheckBigIcon }
	];

	function unpinTask(e: MouseEvent, taskId: string) {
		e.stopPropagation();
		pinnedStore.unpin(taskId);
	}
</script>

<nav class="flex flex-col gap-0.5">
	{#each views as view (view.id)}
		{@const ViewIcon = view.icon}
		<button
			class="group flex items-center rounded-lg text-[13px] transition-all duration-150
				{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-1.5'}
				{contextsStore.activeView === view.id
				? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
				: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
			onclick={() => contextsStore.setView(view.id)}
			title={collapsed ? view.label : undefined}
		>
			<ViewIcon class="h-3.5 w-3.5 shrink-0 opacity-60" />
			{#if !collapsed}
				{view.label}
			{/if}
		</button>
	{/each}

	<!-- Pinned Tasks -->
	{#if pinnedStore.items.length > 0}
		<div class="my-3 border-t border-sidebar-border"></div>

		{#if !collapsed}
			<p class="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
				Закреплённое
			</p>
		{/if}

		{#each pinnedStore.items as pinned (pinned.id)}
			<button
				class="group flex items-center rounded-lg text-[13px] transition-all duration-150
					{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-1.5'}
					text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground"
				onclick={() => pinnedStore.selectTask(pinned.id)}
				title={collapsed ? pinned.content : undefined}
			>
				<PinIcon class="h-3.5 w-3.5 shrink-0 opacity-60" />
				{#if !collapsed}
					<span class="flex-1 truncate text-left">{pinned.content}</span>
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
			</button>
		{/each}
	{/if}

	<!-- Contexts -->
	<div class="my-3 border-t border-sidebar-border"></div>

	{#if !collapsed}
		<p
			class="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60"
		>
			Контекст
		</p>
	{/if}

	{#each contextsStore.contexts as ctx (ctx.id)}
		<button
			class="group flex items-center rounded-lg text-[13px] transition-all duration-150
				{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-1.5'}
				{contextsStore.activeContextId === ctx.id
				? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
				: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
			onclick={() => contextsStore.setContext(ctx.id)}
			title={collapsed ? ctx.display_name : undefined}
		>
			<span class="flex h-3.5 w-3.5 shrink-0 items-center justify-center">
				<span
					class="h-1.5 w-1.5 rounded-full transition-colors duration-150
						{contextsStore.activeContextId === ctx.id ? 'bg-primary' : 'bg-muted-foreground/40'}"
				></span>
			</span>
			{#if !collapsed}
				{ctx.display_name}
			{/if}
		</button>
	{/each}

	<button
		class="group flex items-center rounded-lg text-[13px] transition-all duration-150
			{collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-1.5'}
			{contextsStore.activeContextId === null
			? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
			: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
		onclick={() => contextsStore.setContext(null)}
		title={collapsed ? 'Все' : undefined}
	>
		<LayersIcon class="h-3.5 w-3.5 shrink-0 opacity-60" />
		{#if !collapsed}
			Все
		{/if}
	</button>
</nav>
