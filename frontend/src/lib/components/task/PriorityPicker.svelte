<script lang="ts">
	import type { Priority } from '$lib/api/types';
	import { PRIORITY_COLOR, PRIORITY_LABEL, PRIORITY_ORDER } from '$lib/utils/priority';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import FlagIcon from 'phosphor-svelte/lib/Flag';

	let { value = $bindable<Priority>('no-priority') }: { value?: Priority } = $props();
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-background px-2.5 text-xs font-medium transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
		aria-label="Priority"
	>
		<FlagIcon
			class={`size-3.5 ${PRIORITY_COLOR[value]}`}
			weight={value === 'no-priority' ? 'regular' : 'fill'}
		/>
		<span>{PRIORITY_LABEL[value]}</span>
	</DropdownMenu.Trigger>
	<DropdownMenu.Content class="min-w-[10rem]">
		{#each PRIORITY_ORDER as p (p)}
			<DropdownMenu.Item onSelect={() => (value = p)} class="gap-2">
				<FlagIcon
					class={`size-3.5 ${PRIORITY_COLOR[p]}`}
					weight={p === 'no-priority' ? 'regular' : 'fill'}
				/>
				<span>{PRIORITY_LABEL[p]}</span>
			</DropdownMenu.Item>
		{/each}
	</DropdownMenu.Content>
</DropdownMenu.Root>
