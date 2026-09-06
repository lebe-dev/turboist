import { render, screen, fireEvent } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { settingsStore } from '$lib/stores/settings.svelte';
import type { Project } from '$lib/api/types';
import SidebarProjectRow from './SidebarProjectRow.svelte';

function project(over: Partial<Project> = {}): Project {
	return {
		id: 7,
		contextId: 1,
		title: 'Deep Reader',
		description: '',
		color: 'blue',
		status: 'open',
		projectType: 'generic',
		isPinned: false,
		pinnedAt: null,
		isPrivate: false,
		labels: [],
		troikiCategory: null,
		createdAt: '2026-01-01T00:00:00.000Z',
		updatedAt: '2026-01-01T00:00:00.000Z',
		...over
	};
}

afterEach(() => {
	settingsStore.clear();
});

describe('SidebarProjectRow', () => {
	it('offers a pin action for an unpinned project', async () => {
		const onTogglePin = vi.fn();
		render(SidebarProjectRow, {
			props: { project: project(), href: '/project/7', active: false, onTogglePin }
		});

		const button = screen.getByRole('button', { name: 'Pin Deep Reader' });
		await fireEvent.click(button);

		expect(onTogglePin).toHaveBeenCalledTimes(1);
	});

	it('offers an unpin action for a pinned project', () => {
		render(SidebarProjectRow, {
			props: {
				project: project({ isPinned: true }),
				href: '/project/7',
				active: false,
				onTogglePin: vi.fn()
			}
		});

		expect(screen.getByRole('button', { name: 'Unpin Deep Reader' })).not.toBeNull();
		expect(screen.queryByRole('button', { name: 'Pin Deep Reader' })).toBeNull();
	});

	it('does not navigate when the pin button is clicked', async () => {
		render(SidebarProjectRow, {
			props: { project: project(), href: '/project/7', active: false, onTogglePin: vi.fn() }
		});

		const button = screen.getByRole('button', { name: 'Pin Deep Reader' });
		const event = new MouseEvent('click', { bubbles: true, cancelable: true });
		button.dispatchEvent(event);

		expect(event.defaultPrevented).toBe(true);
	});

	it('links to the project page and shows its title', () => {
		render(SidebarProjectRow, {
			props: { project: project(), href: '/project/7', active: false, onTogglePin: vi.fn() }
		});

		const link = screen.getByRole('link', { name: /Deep Reader/ });
		expect(link.getAttribute('href')).toBe('/project/7');
	});
});
