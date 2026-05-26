import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import OfflineIndicator from './OfflineIndicator.svelte';
import { outboxStatusStore } from '$lib/offline/outboxStatus.svelte';

describe('OfflineIndicator', () => {
	afterEach(() => {
		outboxStatusStore.online = true;
	});

	it('renders nothing when online', () => {
		outboxStatusStore.online = true;
		render(OfflineIndicator);
		expect(screen.queryByTestId('offline-indicator')).toBeNull();
	});

	it('renders the offline label when offline', () => {
		outboxStatusStore.online = false;
		render(OfflineIndicator);
		const el = screen.getByTestId('offline-indicator');
		expect(el).not.toBeNull();
		expect(el.textContent).toContain('Offline');
	});
});
