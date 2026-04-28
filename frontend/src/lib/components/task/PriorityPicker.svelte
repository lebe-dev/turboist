<script lang="ts">
	import type { Priority } from '$lib/api/types';
	import { PRIORITY_COLOR, PRIORITY_LABEL, PRIORITY_ORDER } from '$lib/utils/priority';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import FlagIcon from 'phosphor-svelte/lib/Flag';

	let { value = $bindable<Priority>('no-priority') }: { value?: Priority } = $props();
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="inline-flex items-center gap-1 rounded px-1.5 py-1 text-xs hover:bg-muted"
		aria-label="Priority"
	>
		<FlagIcon class={`size-3.5 ${PRIORITY_COLOR[value]}`} />
		<span>{PRIORITY_LABEL[value]}</span>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content>
		{#each PRIORITY_ORDER as p (p)}
			<DropdownMenu.Item onSelect={() => (value = p)}>
				<FlagIcon class={`size-3.5 ${PRIORITY_COLOR[p]}`} />
				<span>{PRIORITY_LABEL[p]}</span>
			</DropdownMenu.Item>
		{/each}
	</DropdownMenu.Content>
</DropdownMenu.Root>
