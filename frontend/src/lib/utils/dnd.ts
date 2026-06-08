export const SECTION_MIME = 'application/x-turboist-section';
export const TASK_MIME = 'application/x-turboist-task';

export function setSectionDrag(e: DragEvent, sectionId: number): void {
	if (!e.dataTransfer) return;
	e.dataTransfer.setData(SECTION_MIME, String(sectionId));
	e.dataTransfer.setData('text/plain', `section:${sectionId}`);
	e.dataTransfer.effectAllowed = 'move';
}

// Tracks the task currently being dragged with the mouse. `dataTransfer` does
// not expose the dragged id during `dragover` (only on `drop`), so a drop
// target cannot tell whether it is hovering over the drag source. This module
// state lets a TaskItem skip highlighting itself as a sub-task drop target.
let draggingTaskId: number | null = null;

export function setTaskDrag(e: DragEvent, taskId: number): void {
	draggingTaskId = taskId;
	if (!e.dataTransfer) return;
	e.dataTransfer.setData(TASK_MIME, String(taskId));
	e.dataTransfer.setData('text/plain', `task:${taskId}`);
	e.dataTransfer.effectAllowed = 'move';
}

export function currentDraggingTaskId(): number | null {
	return draggingTaskId;
}

export function clearTaskDrag(): void {
	draggingTaskId = null;
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

// --- Touch drag-and-drop for mobile ---

interface TouchDragState {
	taskId: number;
	ghostEl: HTMLElement;
	startX: number;
	startY: number;
	started: boolean;
	armed: boolean;
	holdTimer: ReturnType<typeof setTimeout> | null;
}

let activeTouchDrag: TouchDragState | null = null;
let highlightedSectionEl: Element | null = null;
const SCROLL_CANCEL_THRESHOLD = 10; // px finger movement before long-press fires → cancel drag, allow scroll
const HOLD_DURATION_MS = 350; // long-press duration before drag is armed

function clearHighlight() {
	if (highlightedSectionEl) {
		(highlightedSectionEl as HTMLElement).dataset.touchDragOver = '';
		delete (highlightedSectionEl as HTMLElement).dataset.touchDragOver;
		highlightedSectionEl.classList.remove('touch-drag-over');
		highlightedSectionEl = null;
	}
}

function armDrag(): void {
	if (!activeTouchDrag) return;
	activeTouchDrag.armed = true;
	activeTouchDrag.holdTimer = null;
	if (typeof navigator !== 'undefined' && typeof navigator.vibrate === 'function') {
		try { navigator.vibrate(15); } catch { /* noop */ }
	}

	const sourceEl = document.querySelector(`[data-task-id="${activeTouchDrag.taskId}"]`) as HTMLElement | null;
	if (!sourceEl) { activeTouchDrag = null; return; }
	const ghost = sourceEl.cloneNode(true) as HTMLElement;
	const rect = sourceEl.getBoundingClientRect();
	ghost.style.cssText = `
		position: fixed;
		left: ${rect.left}px;
		top: ${rect.top}px;
		width: ${rect.width}px;
		opacity: 0.8;
		pointer-events: none;
		z-index: 9999;
		border-radius: 8px;
		box-shadow: 0 8px 24px rgba(0,0,0,0.18);
		transform: scale(1.02);
		transition: none;
	`;
	document.body.appendChild(ghost);
	activeTouchDrag.ghostEl = ghost;
	activeTouchDrag.started = true;
	sourceEl.style.opacity = '0.3';
}

function cancelTouchDrag(): void {
	if (!activeTouchDrag) return;
	if (activeTouchDrag.holdTimer !== null) {
		clearTimeout(activeTouchDrag.holdTimer);
	}
	activeTouchDrag = null;
}

export function initTouchDrag(e: TouchEvent, taskId: number, _sourceEl: HTMLElement): void {
	if (activeTouchDrag) return;
	const touch = e.touches[0];
	const state: TouchDragState = {
		taskId,
		ghostEl: null as unknown as HTMLElement,
		startX: touch.clientX,
		startY: touch.clientY,
		started: false,
		armed: false,
		holdTimer: null
	};
	activeTouchDrag = state;
	state.holdTimer = setTimeout(armDrag, HOLD_DURATION_MS);
}

export function updateTouchDrag(e: TouchEvent): boolean {
	if (!activeTouchDrag) return false;
	const touch = e.touches[0];
	const dx = touch.clientX - activeTouchDrag.startX;
	const dy = touch.clientY - activeTouchDrag.startY;

	if (!activeTouchDrag.armed) {
		// Before long-press fires: any meaningful finger movement = user wants to scroll, abort drag entirely.
		if (Math.sqrt(dx * dx + dy * dy) >= SCROLL_CANCEL_THRESHOLD) {
			cancelTouchDrag();
		}
		return false;
	}

	if (!activeTouchDrag.started) return false;

	e.preventDefault();

	const { ghostEl } = activeTouchDrag;
	ghostEl.style.left = `${touch.clientX - ghostEl.offsetWidth / 2}px`;
	ghostEl.style.top = `${touch.clientY - 30}px`;

	// Detect section under finger
	ghostEl.style.display = 'none';
	const el = document.elementFromPoint(touch.clientX, touch.clientY);
	ghostEl.style.display = '';

	clearHighlight();
	const sectionEl = el?.closest('[data-section-id]') ?? el?.closest('[data-section-root]');
	if (sectionEl && sectionEl !== highlightedSectionEl) {
		sectionEl.classList.add('touch-drag-over');
		highlightedSectionEl = sectionEl;
	}

	return true;
}

export function endTouchDrag(e: TouchEvent): { taskId: number; sectionId: number | null } | null {
	if (!activeTouchDrag) return null;
	const { taskId, ghostEl, started, holdTimer } = activeTouchDrag;
	if (holdTimer !== null) clearTimeout(holdTimer);
	activeTouchDrag = null;

	// Restore source opacity
	const sourceEl = document.querySelector(`[data-task-id="${taskId}"]`) as HTMLElement | null;
	if (sourceEl) sourceEl.style.opacity = '';

	if (!started) return null;

	// Remove ghost
	if (ghostEl?.parentNode) ghostEl.parentNode.removeChild(ghostEl);

	// Find drop target
	const touch = e.changedTouches[0];
	const el = document.elementFromPoint(touch.clientX, touch.clientY);

	clearHighlight();

	const sectionEl = el?.closest('[data-section-id]');
	if (sectionEl) {
		const rawId = sectionEl.getAttribute('data-section-id');
		return { taskId, sectionId: rawId !== null ? Number(rawId) : null };
	}
	if (el?.closest('[data-section-root]')) {
		return { taskId, sectionId: null };
	}
	return null;
}
