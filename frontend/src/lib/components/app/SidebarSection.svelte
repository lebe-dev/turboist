<script lang="ts">
	import type { Snippet } from 'svelte';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import PlusIcon from 'phosphor-svelte/lib/Plus';

	let {
		title,
		collapsible = false,
		defaultOpen = true,
		onAdd,
		children
	}: {
		title: string;
		collapsible?: boolean;
		defaultOpen?: boolean;
		onAdd?: () => void;
		children: Snippet;
	} = $props();

	// svelte-ignore state_referenced_locally
	let open = $state(defaultOpen);
</script>

<div class="px-2 py-1">
	<div class="flex items-center justify-between px-2 py-1 text-xs uppercase tracking-wide text-muted-foreground">
		{#if collapsible}
			<button
				type="button"
				class="flex items-center gap-1 hover:text-foreground"
				onclick={() => (open = !open)}
				aria-expanded={open}
			>
				{#if open}
					<CaretDownIcon class="size-3" />
				{:else}
					<CaretRightIcon class="size-3" />
				{/if}
				<span>{title}</span>
			</button>
		{:else}
			<span>{title}</span>
		{/if}
		{#if onAdd}
			<button
				type="button"
				class="rounded p-0.5 hover:bg-muted hover:text-foreground"
				onclick={onAdd}
				aria-label={`Add to ${title}`}
				title={`Add to ${title}`}
			>
				<PlusIcon class="size-3" />
			</button>
		{/if}
	</div>
	{#if !collapsible || open}
		<div class="flex flex-col gap-px">
			{@render children()}
		</div>
	{/if}
</div>
