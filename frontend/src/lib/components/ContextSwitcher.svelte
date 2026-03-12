<script lang="ts">
	import { contextsStore } from '$lib/stores/contexts.svelte';

	const views = [
		{ id: 'all' as const, label: 'Все задачи' },
		{ id: 'weekly' as const, label: 'На неделе' },
		{ id: 'next-week' as const, label: 'След. неделю' }
	];
</script>

<nav class="flex flex-col gap-1">
	<p class="mb-1 px-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
		Контексты
	</p>

	<button
		class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground {contextsStore.activeContextId ===
		null
			? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
			: 'text-sidebar-foreground'}"
		onclick={() => contextsStore.setContext(null)}
	>
		<span class="h-1.5 w-1.5 rounded-full bg-current opacity-60"></span>
		Все
	</button>

	{#each contextsStore.contexts as ctx (ctx.id)}
		<button
			class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground {contextsStore.activeContextId ===
			ctx.id
				? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
				: 'text-sidebar-foreground'}"
			onclick={() => contextsStore.setContext(ctx.id)}
		>
			<span class="h-1.5 w-1.5 rounded-full bg-current opacity-60"></span>
			{ctx.display_name}
		</button>
	{/each}

	<div class="my-3 border-t border-sidebar-border"></div>

	<p class="mb-1 px-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
		Виды
	</p>

	{#each views as view (view.id)}
		<button
			class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground {contextsStore.activeView ===
			view.id
				? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
				: 'text-sidebar-foreground'}"
			onclick={() => contextsStore.setView(view.id)}
		>
			{view.label}
		</button>
	{/each}
</nav>
