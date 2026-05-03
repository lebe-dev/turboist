export const SECTION_MIME = 'application/x-turboist-section';
export const TASK_MIME = 'application/x-turboist-task';

export function setSectionDrag(e: DragEvent, sectionId: number): void {
	if (!e.dataTransfer) return;
	e.dataTransfer.setData(SECTION_MIME, String(sectionId));
	e.dataTransfer.setData('text/plain', `section:${sectionId}`);
	e.dataTransfer.effectAllowed = 'move';
}

export function setTaskDrag(e: DragEvent, taskId: number): void {
	if (!e.dataTransfer) return;
	e.dataTransfer.setData(TASK_MIME, String(taskId));
	e.dataTransfer.setData('text/plain', `task:${taskId}`);
	e.dataTransfer.effectAllowed = 'move';
}

export function hasDragKind(e: DragEvent, kind: 'section' | 'task'): boolean {
	if (!e.dataTransfer) return false;
	const mime = kind === 'section' ? SECTION_MIME : TASK_MIME;
	return Array.from(e.dataTransfer.types).includes(mime);
}

export function readDraggedSection(e: DragEvent): number | null {
	if (!e.dataTransfer) return null;
	const v = e.dataTransfer.getData(SECTION_MIME);
	const id = Number(v);
	return Number.isFinite(id) && id > 0 ? id : null;
}

export function readDraggedTask(e: DragEvent): number | null {
	if (!e.dataTransfer) return null;
	const v = e.dataTransfer.getData(TASK_MIME);
	const id = Number(v);
	return Number.isFinite(id) && id > 0 ? id : null;
}

// Returns true if the cursor is in the upper half of the rect.
export function isUpperHalf(e: DragEvent, rect: DOMRect): boolean {
	return e.clientY < rect.top + rect.height / 2;
}
