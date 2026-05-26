import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import PendingSyncBadge from './PendingSyncBadge.svelte';
import { outboxStatusStore } from '$lib/offline/outboxStatus.svelte';

describe('PendingSyncBadge', () => {
	afterEach(() => {
		outboxStatusStore.pending = 0;
	});

	it('renders nothing when pending is zero', () => {
		outboxStatusStore.pending = 0;
		render(PendingSyncBadge);
		expect(screen.queryByTestId('pending-sync-badge')).toBeNull();
	});

	it('shows the pending count when greater than zero', () => {
		outboxStatusStore.pending = 3;
		render(PendingSyncBadge);
		const el = screen.getByTestId('pending-sync-badge');
		expect(el).not.toBeNull();
		expect(el.textContent).toContain('3');
	});
});
