<script lang="ts">
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth.svelte';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import ContextSwitcher from './ContextSwitcher.svelte';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import LogOutIcon from '@lucide/svelte/icons/log-out';
	import XIcon from '@lucide/svelte/icons/x';

	let { onClose }: { onClose?: () => void } = $props();

	onMount(() => {
		contextsStore.load().catch(console.error);
	});
</script>

<aside
	class="flex h-screen w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground"
>
	<div class="flex h-12 items-center gap-2.5 border-b border-sidebar-border px-4">
		<ZapIcon class="h-4 w-4 text-primary" fill="currentColor" />
		<span class="text-sm font-bold tracking-widest uppercase text-foreground">Turboist</span>
		{#if onClose}
			<button
				class="ml-auto flex h-7 w-7 items-center justify-center rounded-lg text-muted-foreground transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground md:hidden"
				onclick={onClose}
				aria-label="Close menu"
			>
				<XIcon class="h-4 w-4" />
			</button>
		{/if}
	</div>

	<div class="flex-1 overflow-y-auto px-3 py-4">
		<ContextSwitcher />
	</div>

	<div class="border-t border-sidebar-border p-3">
		<button
			class="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm text-muted-foreground transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
			onclick={() => auth.logout()}
		>
			<LogOutIcon class="h-3.5 w-3.5" />
			Выйти
		</button>
	</div>
</aside>
