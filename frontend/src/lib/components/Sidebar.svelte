<script lang="ts">
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { sidebarStore } from '$lib/stores/sidebar.svelte';
	import ContextSwitcher from './ContextSwitcher.svelte';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import XIcon from '@lucide/svelte/icons/x';
	import PanelLeftCloseIcon from '@lucide/svelte/icons/panel-left-close';
	import PanelLeftOpenIcon from '@lucide/svelte/icons/panel-left-open';

	let { onClose }: { onClose?: () => void } = $props();

	onMount(() => {
		contextsStore.load().catch(console.error);
	});
</script>

<aside
	class="flex h-screen shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground transition-[width] duration-200 ease-out
		{sidebarStore.collapsed ? 'w-12' : 'w-60'}"
>
	<div class="flex h-12 items-center border-b border-sidebar-border {sidebarStore.collapsed ? 'justify-center px-0' : 'gap-2.5 px-4'}">
		{#if !sidebarStore.collapsed}
			<ZapIcon class="h-4 w-4 shrink-0 text-primary" fill="currentColor" />
			<span class="text-sm font-bold tracking-widest uppercase text-foreground">Turboist</span>
		{/if}
		{#if onClose}
			<button
				class="ml-auto flex h-7 w-7 items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground md:hidden"
				onclick={onClose}
				aria-label="Close menu"
			>
				<XIcon class="h-4 w-4" />
			</button>
		{/if}
		<button
			class="{sidebarStore.collapsed ? '' : 'ml-auto'} hidden h-7 w-7 items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground md:flex"
			onclick={() => sidebarStore.toggle()}
			aria-label={sidebarStore.collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
		>
			{#if sidebarStore.collapsed}
				<PanelLeftOpenIcon class="h-3.5 w-3.5" />
			{:else}
				<PanelLeftCloseIcon class="h-3.5 w-3.5" />
			{/if}
		</button>
	</div>

	<div class="flex-1 overflow-y-auto {sidebarStore.collapsed ? 'px-1.5 py-2' : 'px-3 py-4'}">
		<ContextSwitcher collapsed={sidebarStore.collapsed} />
	</div>

	<div class="border-t border-sidebar-border {sidebarStore.collapsed ? 'p-1.5' : 'p-3'}">
		<button
			class="flex w-full items-center rounded-lg text-sm text-muted-foreground transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground
				{sidebarStore.collapsed ? 'justify-center p-2' : 'gap-2.5 px-2.5 py-2'}"
			onclick={() => auth.logout()}
			title={sidebarStore.collapsed ? 'Выйти' : undefined}
		>
			<LogOutIcon class="h-3.5 w-3.5 shrink-0" />
			{#if !sidebarStore.collapsed}
				Выйти
			{/if}
		</button>
	</div>
</aside>
