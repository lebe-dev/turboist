<script lang="ts">
	import { getContext, setContext } from 'svelte';
	import { DAY_PART_DRAG_KEY } from '$lib/context/dependencyDrag';
	import type { DayPartDrag } from '$lib/hooks/useDayPartDrag.svelte';
	import type { Snippet } from 'svelte';
	import type { DayPart } from '$lib/api/types';
	import type { DayPartInterval } from '$lib/utils/viewGroup';
	import DayPartSectionHeader from './DayPartSectionHeader.svelte';

	let {
		part,
		label,
		interval,
		count,
		active = false,
		onBulkMove,
		children
	}: {
		part: DayPart;
		label: string;
		interval: DayPartInterval | null;
		count: number;
		active?: boolean;
		onBulkMove?: (targetPart: DayPart) => void;
		children: Snippet;
	} = $props();

	setContext('dayPartActive', () => active);

	// On Today the whole section takes a row carried from another phase. Rows
	// leave their dragover unhandled there, so it reaches the section.
	const drag = getContext<DayPartDrag | undefined>(DAY_PART_DRAG_KEY);
	const landing = $derived(!!drag && drag.landing === part);

	function onDragOver(e: DragEvent): void {
		if (!drag || drag.draggedId === null) return;
		drag.over(part);
		// Leaving the browser's "no drop" cursor over the row's own phase says
		// the drop would change nothing.
		if (part === drag.origin) return;
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
	}

	function onDragLeave(e: DragEvent): void {
		const related = e.relatedTarget as Node | null;
		if (related && (e.currentTarget as HTMLElement).contains(related)) return;
		if (drag?.hover === part) drag.over(null);
	}

	function onDrop(e: DragEvent): void {
		if (!drag) return;
		e.preventDefault();
		drag.drop(part);
	}
</script>

<section
	class="rounded-lg border px-1 py-2 transition-colors {active ? 'border-foreground/25' : 'border-transparent'}"
	class:bg-accent={landing}
	class:ring-2={landing}
	class:ring-primary={landing}
	data-day-part-drop={drag ? part : undefined}
	ondragover={drag ? onDragOver : undefined}
	ondragleave={drag ? onDragLeave : undefined}
	ondrop={drag ? onDrop : undefined}
	role={drag ? 'region' : undefined}
	aria-label={drag ? label : undefined}
>
	<DayPartSectionHeader {part} {label} {interval} {count} {active} {onBulkMove} />
	{@render children()}
</section>
