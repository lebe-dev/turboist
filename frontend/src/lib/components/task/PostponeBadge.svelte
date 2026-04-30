<script lang="ts">
	import ClockClockwiseIcon from 'phosphor-svelte/lib/ClockClockwise';

	let { count }: { count: number } = $props();
</script>

{#if count >= 2}
	<span
		class="inline-flex items-center gap-1 text-xs font-medium"
		class:text-amber-600={count === 2}
		class:text-red-600={count > 2}
		title="Postponed {count} times"
	>
		<ClockClockwiseIcon class="size-3.5 shrink-0" />
		<span>{count}</span>
	</span>
{/if}
