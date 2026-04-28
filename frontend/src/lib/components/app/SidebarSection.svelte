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

<div class="px-2 pb-1 pt-3">
	<div class="flex items-center justify-between gap-1 px-2.5 pb-1.5">
		{#if collapsible}
			<button
				type="button"
				class="group flex flex-1 items-center gap-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-muted-foreground transition-colors hover:text-foreground"
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
			<span class="flex-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
				{title}
			</span>
		{/if}
		{#if onAdd}
			<button
				type="button"
				class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				onclick={onAdd}
				aria-label={`Add to ${title}`}
				title={`Add to ${title}`}
			>
				<PlusIcon class="size-3.5" />
			</button>
		{/if}
	</div>
	{#if !collapsible || open}
		<div class="flex flex-col gap-0.5">
			{@render children()}
		</div>
	{/if}
</div>
