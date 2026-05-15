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

// --- Touch drag-and-drop for mobile ---

interface TouchDragState {
	taskId: number;
	ghostEl: HTMLElement;
	startX: number;
	startY: number;
	started: boolean;
}

let activeTouchDrag: TouchDragState | null = null;
let highlightedSectionEl: Element | null = null;
const DRAG_THRESHOLD = 8; // px before drag starts

function clearHighlight() {
	if (highlightedSectionEl) {
		(highlightedSectionEl as HTMLElement).dataset.touchDragOver = '';
		delete (highlightedSectionEl as HTMLElement).dataset.touchDragOver;
		highlightedSectionEl.classList.remove('touch-drag-over');
		highlightedSectionEl = null;
	}
}

export function initTouchDrag(e: TouchEvent, taskId: number, sourceEl: HTMLElement): void {
	if (activeTouchDrag) return;
	const touch = e.touches[0];
	activeTouchDrag = {
		taskId,
		ghostEl: null as unknown as HTMLElement,
		startX: touch.clientX,
		startY: touch.clientY,
		started: false
	};
}

export function updateTouchDrag(e: TouchEvent): boolean {
	if (!activeTouchDrag) return false;
	const touch = e.touches[0];

	if (!activeTouchDrag.started) {
		const dx = touch.clientX - activeTouchDrag.startX;
		const dy = touch.clientY - activeTouchDrag.startY;
		if (Math.sqrt(dx * dx + dy * dy) < DRAG_THRESHOLD) return false;

		// Start dragging: create ghost
		const sourceEl = document.querySelector(`[data-task-id="${activeTouchDrag.taskId}"]`) as HTMLElement | null;
		if (!sourceEl) { activeTouchDrag = null; return false; }

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
	const { taskId, ghostEl, started } = activeTouchDrag;
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
