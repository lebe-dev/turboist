<script lang="ts">
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import ProhibitIcon from 'phosphor-svelte/lib/Prohibit';
	import type { Task } from '$lib/api/types';
	import type { DependencyDrag } from '$lib/hooks/useDependencyDrag.svelte';
	import { stripMarkdownSyntax } from '$lib/utils/markdown';
	import { t } from '$lib/i18n';

	let { drag, tasks }: { drag: DependencyDrag; tasks: readonly Task[] } = $props();

	const EDGE = 8;
	// Above the pointer rather than below it, so a finger does not cover the text.
	const LIFT = 52;

	let width = $state(0);

	const hover = $derived(drag.hover);
	const target = $derived(hover ? tasks.find((t) => t.id === hover.targetId) : undefined);
	const refusal = $derived(hover ? drag.refusalFor(hover.targetId) : null);
	const title = $derived(target ? stripMarkdownSyntax(target.title) : '');
	const message = $derived.by(() => {
		const values = { title };
		switch (refusal) {
			case 'ancestor':
				return $t('page.task.dependencyDrag.refusedAncestor');
			case 'descendant':
				return $t('page.task.dependencyDrag.refusedDescendant');
			case 'completed':
				return $t('page.task.dependencyDrag.refusedCompleted', { values });
			case 'exists':
				return $t('page.task.dependencyDrag.refusedExists', { values });
			case 'cycle':
				return $t('page.task.dependencyDrag.refusedCycle', { values });
			default:
				return $t('page.task.dependencyDrag.willDepend', { values });
		}
	});
	const left = $derived.by(() => {
		if (!hover) return 0;
		const viewport = typeof window === 'undefined' ? Infinity : window.innerWidth;
		return Math.max(EDGE, Math.min(hover.x - width / 2, viewport - width - EDGE));
	});
	const top = $derived(hover ? Math.max(EDGE, hover.y - LIFT) : 0);
</script>

{#if hover && target}
	<div
		bind:clientWidth={width}
		role="status"
		aria-live="polite"
		class="pointer-events-none fixed z-[10000] flex max-w-[min(22rem,calc(100vw-1rem))] items-center gap-2 rounded-md border px-2.5 py-1.5 text-xs font-medium shadow-lg {refusal
			? 'border-destructive/40 bg-popover text-destructive'
			: 'border-border bg-popover text-popover-foreground'}"
		style:left={`${left}px`}
		style:top={`${top}px`}
		data-testid="dependency-drag-tooltip"
	>
		{#if refusal}
			<ProhibitIcon class="size-3.5 shrink-0" weight="bold" />
		{:else}
			<LockSimpleIcon class="size-3.5 shrink-0 text-primary" weight="fill" />
		{/if}
		<span class="truncate">{message}</span>
	</div>
{/if}
