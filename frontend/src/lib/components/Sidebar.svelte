<script lang="ts">
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { contextsStore } from '$lib/stores/contexts';
	import ContextSwitcher from './ContextSwitcher.svelte';

	onMount(() => {
		contextsStore.load().catch(console.error);
	});
</script>

<aside
	class="flex h-screen w-56 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground"
>
	<div class="flex h-14 items-center border-b border-sidebar-border px-4">
		<span class="text-base font-semibold">Turboist</span>
	</div>

	<div class="flex-1 overflow-y-auto p-3">
		<ContextSwitcher />
	</div>

	<div class="border-t border-sidebar-border p-3">
		<button
			class="w-full rounded-md px-2 py-1.5 text-left text-sm text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
			onclick={() => auth.logout()}
		>
			Выйти
		</button>
	</div>
</aside>
