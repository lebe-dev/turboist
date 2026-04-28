<script lang="ts">
	import { COLOR_PALETTE } from './colorPalette';

	let { value = $bindable<string>('') }: { value: string } = $props();
</script>

<div class="flex flex-wrap gap-1.5">
	{#each COLOR_PALETTE as opt (opt.token)}
		{@const active = value === opt.token}
		<button
			type="button"
			class="size-6 rounded-full border-2 transition"
			class:border-foreground={active}
			class:border-transparent={!active}
			style={`background-color: ${opt.token}`}
			aria-label={opt.label}
			aria-pressed={active}
			title={opt.label}
			onclick={() => (value = opt.token)}
		></button>
	{/each}
</div>
