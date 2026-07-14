import { render, fireEvent } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { createRawSnippet, tick } from 'svelte';
import PullToRefresh from './PullToRefresh.svelte';

function textChildren(text: string) {
	return createRawSnippet(() => ({
		render: () => `<span data-testid="content">${text}</span>`
	}));
}

// jsdom has no Touch/TouchEvent constructors, so build a plain Event and
// attach a `touches` list by hand — the component only ever reads
// `e.touches[0].clientY`.
function touchEvent(type: string, clientY: number): Event {
	const e = new Event(type, { bubbles: true, cancelable: true });
	Object.defineProperty(e, 'touches', { value: [{ clientY }] });
	return e;
}

describe('PullToRefresh', () => {
	it('refreshes when pulled past the threshold from the top', async () => {
		const onRefresh = vi.fn().mockResolvedValue(undefined);
		const { container } = render(PullToRefresh, {
			props: { onRefresh, class: 'root', children: textChildren('items') }
		});
		const root = container.querySelector('.root')!;
		expect(root.scrollTop).toBe(0);

		await fireEvent(root, touchEvent('touchstart', 0));
		await fireEvent(root, touchEvent('touchmove', 300));
		await fireEvent(root, touchEvent('touchend', 300));
		await tick();

		expect(onRefresh).toHaveBeenCalledOnce();
	});

	it('snaps back without refreshing when the pull stays under the threshold', async () => {
		const onRefresh = vi.fn().mockResolvedValue(undefined);
		const { container } = render(PullToRefresh, {
			props: { onRefresh, class: 'root', children: textChildren('items') }
		});
		const root = container.querySelector('.root')!;

		await fireEvent(root, touchEvent('touchstart', 0));
		await fireEvent(root, touchEvent('touchmove', 50));
		await fireEvent(root, touchEvent('touchend', 50));
		await tick();

		expect(onRefresh).not.toHaveBeenCalled();
	});

	it('ignores the gesture when the container is not scrolled to the top', async () => {
		const onRefresh = vi.fn().mockResolvedValue(undefined);
		const { container } = render(PullToRefresh, {
			props: { onRefresh, class: 'root', children: textChildren('items') }
		});
		const root = container.querySelector('.root')!;
		Object.defineProperty(root, 'scrollTop', { value: 10, configurable: true });

		await fireEvent(root, touchEvent('touchstart', 0));
		await fireEvent(root, touchEvent('touchmove', 300));
		await fireEvent(root, touchEvent('touchend', 300));
		await tick();

		expect(onRefresh).not.toHaveBeenCalled();
	});

	it('ignores an upward swipe', async () => {
		const onRefresh = vi.fn().mockResolvedValue(undefined);
		const { container } = render(PullToRefresh, {
			props: { onRefresh, class: 'root', children: textChildren('items') }
		});
		const root = container.querySelector('.root')!;

		await fireEvent(root, touchEvent('touchstart', 300));
		await fireEvent(root, touchEvent('touchmove', 0));
		await fireEvent(root, touchEvent('touchend', 0));
		await tick();

		expect(onRefresh).not.toHaveBeenCalled();
	});
});
