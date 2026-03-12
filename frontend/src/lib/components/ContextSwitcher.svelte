<script lang="ts">
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import LayersIcon from '@lucide/svelte/icons/layers';
	import ListIcon from '@lucide/svelte/icons/list';
	import CalendarDaysIcon from '@lucide/svelte/icons/calendar-days';
	import CalendarClockIcon from '@lucide/svelte/icons/calendar-clock';

	const views = [
		{ id: 'all' as const, label: 'Все задачи', icon: ListIcon },
		{ id: 'weekly' as const, label: 'На неделе', icon: CalendarDaysIcon },
		{ id: 'next-week' as const, label: 'След. неделю', icon: CalendarClockIcon }
	];
</script>

<nav class="flex flex-col gap-0.5">
	<p class="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider transition-colors duration-150
		{contextsStore.activeContextId !== null ? 'text-primary' : 'text-muted-foreground/60'}">
		Контексты
	</p>

	<button
		class="group flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-[13px] transition-all duration-150
			{contextsStore.activeContextId === null
			? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
			: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
		onclick={() => contextsStore.setContext(null)}
	>
		<LayersIcon class="h-3.5 w-3.5 shrink-0 opacity-60" />
		Все
	</button>

	{#each contextsStore.contexts as ctx (ctx.id)}
		<button
			class="group flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-[13px] transition-all duration-150
				{contextsStore.activeContextId === ctx.id
				? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
				: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
			onclick={() => contextsStore.setContext(ctx.id)}
		>
			<span class="flex h-3.5 w-3.5 shrink-0 items-center justify-center">
				<span
					class="h-1.5 w-1.5 rounded-full transition-colors duration-150
						{contextsStore.activeContextId === ctx.id ? 'bg-primary' : 'bg-muted-foreground/40'}"
				></span>
			</span>
			{ctx.display_name}
		</button>
	{/each}

	<div class="my-3 border-t border-sidebar-border"></div>

	<p class="mb-1.5 px-2.5 text-[11px] font-semibold uppercase tracking-wider transition-colors duration-150
		{contextsStore.activeView !== 'all' ? 'text-primary' : 'text-muted-foreground/60'}">
		Виды
	</p>

	{#each views as view (view.id)}
		{@const ViewIcon = view.icon}
		<button
			class="group flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-[13px] transition-all duration-150
				{contextsStore.activeView === view.id
				? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
				: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground'}"
			onclick={() => contextsStore.setView(view.id)}
		>
			<ViewIcon class="h-3.5 w-3.5 shrink-0 opacity-60" />
			{view.label}
		</button>
	{/each}
</nav>
