<script lang="ts">
	import { hasDragKind, readDraggedTask } from '$lib/utils/dnd';
	import { t } from '$lib/i18n';

	let { onDetach }: { onDetach: (id: number) => void } = $props();
	let active = $state(false);
</script>

<div
	data-task-detach="true"
	role="list"
	class="my-2 rounded-md border border-dashed border-primary/50 px-3 py-3 text-sm text-muted-foreground transition-colors"
	class:bg-accent={active}
	ondragover={(e) => {
		if (!hasDragKind(e, 'task')) return;
		e.preventDefault();
		e.stopPropagation();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
		active = true;
	}}
	ondragleave={(e) => {
		if (e.relatedTarget instanceof Node && e.currentTarget.contains(e.relatedTarget)) return;
		active = false;
	}}
	ondrop={(e) => {
		e.preventDefault();
		e.stopPropagation();
		active = false;
		const id = readDraggedTask(e);
		if (id !== null) onDetach(id);
	}}
>
	{$t('page.task.detachDrag.hint')}
</div>
