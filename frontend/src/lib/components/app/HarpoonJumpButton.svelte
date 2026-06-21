<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/i18n';
	import { harpoonStore } from '$lib/stores/harpoon.svelte';
	import type { HarpoonKind } from '$lib/api/types';
	import ArrowULeftDownIcon from 'phosphor-svelte/lib/ArrowULeftDown';
	import ArrowULeftUpIcon from 'phosphor-svelte/lib/ArrowULeftUp';

	let { kind, id }: { kind: HarpoonKind; id: number } = $props();

	const target = $derived(harpoonStore.target(kind, id));

	function jump(): void {
		if (!target) return;
		const path =
			target.slot.kind === 'task'
				? resolve('/(app)/task/[id]', { id: String(target.slot.id) })
				: resolve('/(app)/project/[id]', { id: String(target.slot.id) });
		void goto(path);
	}
</script>

{#if target}
	<Button
		variant="ghost"
		size="sm"
		class="size-8 p-0 text-muted-foreground hover:text-foreground"
		aria-label={$t('harpoon.jumpTo', { values: { title: target.slot.title } })}
		title={$t('harpoon.jumpTo', { values: { title: target.slot.title } })}
		onclick={jump}
	>
		{#if target.direction === 'down'}
			<ArrowULeftDownIcon class="size-5" />
		{:else}
			<ArrowULeftUpIcon class="size-5" />
		{/if}
	</Button>
{/if}
