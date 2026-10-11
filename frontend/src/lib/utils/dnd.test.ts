import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cancelTouchDrag, endTouchDrag, initTouchDrag, updateTouchDrag } from './dnd';

// jsdom has no TouchEvent constructor: build a plain Event carrying the touch
// lists the module reads.
function touchEvent(type: string, x: number, y: number, touches = 1): TouchEvent {
	const e = new Event(type, { bubbles: true, cancelable: true });
	const list = Array.from({ length: touches }, () => ({ clientX: x, clientY: y }));
	Object.defineProperty(e, 'touches', { value: list });
	Object.defineProperty(e, 'changedTouches', { value: [{ clientX: x, clientY: y }] });
	return e as unknown as TouchEvent;
}

function scrollBlocked(): boolean {
	const move = touchEvent('touchmove', 0, 0);
	window.dispatchEvent(move);
	return move.defaultPrevented;
}

function ghosts(): number {
	// The ghost is the only body-level clone of the row.
	return document.body.querySelectorAll(':scope > .row').length;
}

let row: HTMLElement;

function armDragOn(el: HTMLElement): void {
	initTouchDrag(touchEvent('touchstart', 10, 10), 1, el);
	vi.advanceTimersByTime(400);
}

beforeEach(() => {
	vi.useFakeTimers();
	document.elementFromPoint = () => null;
	row = document.createElement('div');
	row.className = 'row';
	row.dataset.taskId = '1';
	const list = document.createElement('div');
	list.appendChild(row);
	document.body.appendChild(list);
});

afterEach(() => {
	cancelTouchDrag();
	document.body.innerHTML = '';
	vi.useRealTimers();
});

describe('touch drag', () => {
	it('blocks scrolling while armed and releases it on touchend', () => {
		armDragOn(row);
		expect(scrollBlocked()).toBe(true);
		expect(ghosts()).toBe(1);
		updateTouchDrag(touchEvent('touchmove', 20, 40));
		endTouchDrag(touchEvent('touchend', 20, 40));
		expect(scrollBlocked()).toBe(false);
		expect(ghosts()).toBe(0);
		expect(row.style.opacity).toBe('');
	});

	it('releases scrolling when the touch is cancelled by the system', () => {
		armDragOn(row);
		window.dispatchEvent(touchEvent('touchcancel', 0, 0, 0));
		expect(scrollBlocked()).toBe(false);
		expect(ghosts()).toBe(0);
	});

	it('recovers on the next touch when the touchend was lost', () => {
		armDragOn(row);
		// touchend went to a detached row and never reached any handler.
		window.dispatchEvent(touchEvent('touchstart', 5, 5));
		expect(scrollBlocked()).toBe(false);
		expect(ghosts()).toBe(0);
		// And a fresh drag can start again.
		armDragOn(row);
		expect(scrollBlocked()).toBe(true);
	});

	it('releases scrolling when the app goes to the background', () => {
		armDragOn(row);
		Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
		document.dispatchEvent(new Event('visibilitychange'));
		Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
		expect(scrollBlocked()).toBe(false);
	});
});
