<script lang="ts">
	import FunnelIcon from 'phosphor-svelte/lib/Funnel';
	import XIcon from 'phosphor-svelte/lib/X';
	import { toast } from 'svelte-sonner';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { userStateStore } from '$lib/stores/userState.svelte';

	const activeId = $derived(userStateStore.activeContextId);
	const activeContext = $derived(
		activeId == null ? null : (contextsStore.items.find((c) => c.id === activeId) ?? null)
	);

	async function clear(): Promise<void> {
		try {
			await userStateStore.setActiveContextId(null);
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Failed to clear context';
			toast.error(message);
		}
	}
</script>

{#if activeContext}
	<div
		class="flex items-center justify-between gap-3 border-b border-amber-500/30 bg-amber-500/10 px-4 py-2 text-xs text-amber-700 dark:text-amber-400 sm:px-6"
		role="status"
	>
		<div class="flex min-w-0 items-center gap-2">
			<FunnelIcon class="size-3.5 shrink-0" weight="fill" />
			<span class="min-w-0 truncate">
				Filter active: context
				<span class="inline-flex items-center gap-1 font-semibold">
					{#if activeContext.color}
						<span
							class="size-1.5 rounded-full"
							style={`background-color: ${activeContext.color}`}
						></span>
					{/if}
					{activeContext.name}
				</span>
				· some tasks may be hidden.
			</span>
		</div>
		<button
			type="button"
			onclick={clear}
			class="inline-flex shrink-0 items-center gap-1 rounded-full border border-amber-500/40 px-2 py-0.5 font-medium transition-colors hover:bg-amber-500/20"
		>
			<XIcon class="size-3" />
			Clear filter
		</button>
	</div>
{/if}
