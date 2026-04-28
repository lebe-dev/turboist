<script lang="ts">
	import { mode, toggleMode } from 'mode-watcher';
	import SunIcon from 'phosphor-svelte/lib/Sun';
	import MoonIcon from 'phosphor-svelte/lib/Moon';
	import { Button } from '$lib/components/ui/button';
</script>

<Button
	variant="ghost"
	size="icon"
	onclick={() => toggleMode()}
	aria-label="Toggle theme"
	title="Toggle theme"
>
	{#if mode.current === 'dark'}
		<MoonIcon class="size-4" />
	{:else}
		<SunIcon class="size-4" />
	{/if}
</Button>
