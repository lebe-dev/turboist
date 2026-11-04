<script lang="ts">
	import type { DayPart } from '$lib/api/types';
	import type { DayPartDrag } from '$lib/hooks/useDayPartDrag.svelte';
	import type { DayPartGroupMeta } from '$lib/utils/viewGroup';
	import { iconFor } from './dayPartIcon';
	import { t } from '$lib/i18n';

	// Every phase of the day, floated over the top of the page while a row is
	// carried. The page leaves out a phase nothing falls into, so without the bar
	// an empty evening could not be dropped onto at all; and a long day pushes
	// most phases off screen. It floats rather than sitting in the flow, so
	// appearing never shifts the rows under the pointer.
	let { drag, parts }: { drag: DayPartDrag; parts: DayPartGroupMeta[] } = $props();

	function onDragOver(e: DragEvent, part: DayPart): void {
		if (drag.draggedId === null) return;
		e.stopPropagation();
		drag.over(part);
		if (part === drag.origin) return;
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
	}

	function onDragLeave(e: DragEvent, part: DayPart): void {
		const related = e.relatedTarget as Node | null;
		if (related && (e.currentTarget as HTMLElement).contains(related)) return;
		if (drag.hover === part) drag.over(null);
	}

	function onDrop(e: DragEvent, part: DayPart): void {
		e.preventDefault();
		e.stopPropagation();
		drag.drop(part);
	}
</script>

<div class="pointer-events-none fixed inset-x-0 top-2 z-50 flex justify-center px-2">
	<div
		class="pointer-events-auto w-full max-w-xl rounded-xl border bg-popover p-2 shadow-lg"
		data-testid="day-part-drop-bar"
	>
		<div class="px-2 pb-1.5 text-xs text-muted-foreground">{$t('page.today.dayPartDropHint')}</div>
		<div class="grid grid-cols-4 gap-1">
			{#each parts as meta (meta.part)}
				{@const Icon = iconFor(meta.part)}
				{@const landing = drag.landing === meta.part}
				<div
					class="flex flex-col items-center gap-1 rounded-lg px-1 py-2 text-center text-xs transition-colors"
					class:bg-accent={landing}
					class:text-foreground={landing}
					class:text-muted-foreground={!landing}
					class:opacity-45={drag.origin === meta.part}
					data-day-part-drop={meta.part}
					ondragover={(e) => onDragOver(e, meta.part)}
					ondragleave={(e) => onDragLeave(e, meta.part)}
					ondrop={(e) => onDrop(e, meta.part)}
					role="region"
					aria-label={meta.label}
				>
					<Icon class="size-5" />
					<span>{meta.label}</span>
				</div>
			{/each}
		</div>
	</div>
</div>
