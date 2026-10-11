import { dropModeForOffset, type DropMode } from './dependencyDrop';

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

/** The active page observes row drops for dependencies and nesting. */
export interface TouchTaskDragListener {
	begin(taskId: number): void;
	over(targetId: number | null, mode: DropMode | null, x: number, y: number): void;
	drop(targetId: number | null, mode: DropMode | null): void;
	detach?(taskId: number): void;
}

let touchTaskListener: TouchTaskDragListener | null = null;

export function setTouchTaskDragListener(listener: TouchTaskDragListener): () => void {
	touchTaskListener = listener;
	return () => {
		if (touchTaskListener === listener) touchTaskListener = null;
	};
}

function taskIdAt(el: Element | null): number | null {
	const raw = el?.closest('[data-task-id]')?.getAttribute('data-task-id');
	const id = Number(raw);
	return raw && Number.isFinite(id) ? id : null;
}

// Finds the task row under the finger and where inside its height the finger
// sits, so the touch path can offer the same dependency/nest split the mouse
// path computes from `getBoundingClientRect` on the row's own dragover.
function taskRowHitAt(el: Element | null, clientY: number): { targetId: number; mode: DropMode } | null {
	const rowEl = el?.closest('[data-task-id]') ?? null;
	const targetId = taskIdAt(rowEl);
	if (!rowEl || targetId === null) return null;
	const rect = rowEl.getBoundingClientRect();
	const relativeY = rect.height > 0 ? (clientY - rect.top) / rect.height : 0.5;
	return { targetId, mode: dropModeForOffset(relativeY) };
}

// Svelte registers touchmove handlers as passive, so the preventDefault in
// updateTouchDrag cannot stop the page from scrolling under an armed drag. A
// non-passive window listener, held only while a drag is armed, can.
function blockScroll(e: TouchEvent): void {
	if (e.cancelable) e.preventDefault();
}
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
	// Keep the ghost out of `[data-task-id]` lookups and hit-testing.
	ghost.removeAttribute('data-task-id');
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
	window.addEventListener('touchmove', blockScroll, { passive: false });
	touchTaskListener?.begin(activeTouchDrag.taskId);
}

// Undoes everything a touch drag may have set up: hold timer, ghost, faded
// source row, section highlight and — crucially — the window-level scroll
// blocker. Safe to call at any phase and more than once.
function teardownTouchDrag(): TouchDragState | null {
	const state = activeTouchDrag;
	if (!state) return null;
	activeTouchDrag = null;
	if (state.holdTimer !== null) clearTimeout(state.holdTimer);
	window.removeEventListener('touchmove', blockScroll);
	if (state.ghostEl?.parentNode) state.ghostEl.parentNode.removeChild(state.ghostEl);
	const sourceEl = document.querySelector(`[data-task-id="${state.taskId}"]`) as HTMLElement | null;
	if (sourceEl) sourceEl.style.opacity = '';
	clearHighlight();
	return state;
}

// Aborts a drag without dropping anything (touchcancel, app backgrounded,
// stale gesture). Lets the page drop its own drag highlight too.
export function cancelTouchDrag(): void {
	const state = teardownTouchDrag();
	if (state?.started) touchTaskListener?.drop(null, null);
}

// The row's own touchend/touchcancel handlers are not enough on their own:
// Android (Capacitor WebView) sends touchcancel when a system gesture or the
// long-press itself steals the touch, and when the row re-renders mid-drag
// (SSE update, list reorder) the remaining events go to the detached node and
// never reach any handler. Either way the armed drag used to stay alive — the
// ghost hung on screen and the window touchmove blocker killed scrolling
// until the app was restarted. These window listeners guarantee cleanup.
let safetyNetInstalled = false;
function installTouchDragSafetyNet(): void {
	if (safetyNetInstalled || typeof window === 'undefined') return;
	safetyNetInstalled = true;
	// Capture phase runs before the row's touchstart, so a stale drag left by
	// a lost touchend is cleared before a new one is initialised.
	window.addEventListener(
		'touchstart',
		(e) => {
			if (activeTouchDrag && e.touches.length === 1) cancelTouchDrag();
		},
		{ capture: true, passive: true }
	);
	window.addEventListener('touchcancel', () => cancelTouchDrag(), { capture: true, passive: true });
	window.addEventListener('blur', () => cancelTouchDrag());
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState === 'hidden') cancelTouchDrag();
	});
}

export function initTouchDrag(e: TouchEvent, taskId: number, _sourceEl: HTMLElement): void {
	installTouchDragSafetyNet();
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
	const sectionEl = el?.closest('[data-task-detach]') ??
		(!taskRowHitAt(el, touch.clientY) || !touchTaskListener
			? el?.closest('[data-section-id]') ?? el?.closest('[data-section-root]')
			: null);
	if (sectionEl && sectionEl !== highlightedSectionEl) {
		sectionEl.classList.add('touch-drag-over');
		highlightedSectionEl = sectionEl;
	}
	const hit = taskRowHitAt(el, touch.clientY);
	touchTaskListener?.over(hit?.targetId ?? null, hit?.mode ?? null, touch.clientX, touch.clientY);

	return true;
}

export function endTouchDrag(e: TouchEvent): { taskId: number; sectionId: number | null } | null {
	const state = teardownTouchDrag();
	if (!state?.started) return null;
	const { taskId } = state;

	// Find drop target
	const touch = e.changedTouches[0];
	if (!touch) {
		touchTaskListener?.drop(null, null);
		return null;
	}
	const el = document.elementFromPoint(touch.clientX, touch.clientY);

	const hit = taskRowHitAt(el, touch.clientY);
	touchTaskListener?.drop(hit?.targetId ?? null, hit?.mode ?? null);
	if (hit && touchTaskListener) return null;
	if (el?.closest('[data-task-detach]')) {
		touchTaskListener?.detach?.(taskId);
		return null;
	}

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
