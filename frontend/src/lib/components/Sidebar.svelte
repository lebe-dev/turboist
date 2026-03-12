<script lang="ts">
	import { onMount } from 'svelte';
	import { auth } from '$lib/stores/auth';
	import { contextsStore } from '$lib/stores/contexts';
	import ContextSwitcher from './ContextSwitcher.svelte';

	let { onClose }: { onClose?: () => void } = $props();

	onMount(() => {
		contextsStore.load().catch(console.error);
	});
</script>

<aside
	class="flex h-screen w-56 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground"
>
	<div class="flex h-14 items-center border-b border-sidebar-border px-4">
		<span class="text-base font-semibold">Turboist</span>
		{#if onClose}
			<button
				class="ml-auto flex h-8 w-8 items-center justify-center rounded-md hover:bg-sidebar-accent md:hidden"
				onclick={onClose}
				aria-label="Закрыть меню"
			>
				<svg
					xmlns="http://www.w3.org/2000/svg"
					width="18"
					height="18"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<line x1="18" y1="6" x2="6" y2="18" />
					<line x1="6" y1="6" x2="18" y2="18" />
				</svg>
			</button>
		{/if}
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
