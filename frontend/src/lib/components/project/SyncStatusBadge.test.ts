import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import SyncStatusBadge from './SyncStatusBadge.svelte';
import type { SyncStatus } from '$lib/api/types';

function status(over: Partial<SyncStatus> = {}): SyncStatus {
	return {
		projectId: 7,
		status: 'synced',
		pendingCount: 0,
		unreachablePeer: '',
		keyMismatchPeer: '',
		...over
	};
}

describe('SyncStatusBadge (Federation v1 F4.3, US-4.3)', () => {
	it('renders nothing when there is no status (non-federated project)', () => {
		const { container } = render(SyncStatusBadge, { status: undefined });
		expect(container.textContent?.trim()).toBe('');
	});

	it('renders the green synced label (US-4.3 AC1)', () => {
		render(SyncStatusBadge, { status: status({ status: 'synced' }) });
		expect(screen.getByText('In sync')).toBeTruthy();
	});

	it('renders the yellow pending label with the change count (US-4.3 AC2)', () => {
		render(SyncStatusBadge, { status: status({ status: 'pending', pendingCount: 3 }) });
		// The count is interpolated into the label.
		expect(screen.getByText(/3 changes pending/)).toBeTruthy();
	});

	it('renders the orange unreachable label naming the peer (US-4.3 AC3)', () => {
		render(
			SyncStatusBadge,
			{ status: status({ status: 'unreachable', unreachablePeer: 'https://bob.example' }) }
		);
		expect(screen.getByText('Peer unreachable')).toBeTruthy();
		// The offending peer URL surfaces in the title/tooltip.
		const badge = screen.getByText('Peer unreachable').closest('[title]');
		expect(badge?.getAttribute('title')).toContain('https://bob.example');
	});

	it('renders the red key-mismatch label naming the peer (US-4.3 AC4)', () => {
		render(
			SyncStatusBadge,
			{ status: status({ status: 'key_mismatch', keyMismatchPeer: 'https://bob.example' }) }
		);
		expect(screen.getByText('Key mismatch')).toBeTruthy();
		const badge = screen.getByText('Key mismatch').closest('[title]');
		expect(badge?.getAttribute('title')).toContain('https://bob.example');
		expect(badge?.getAttribute('title')).toContain('manual action');
	});
});
