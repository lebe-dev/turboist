import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import { createRawSnippet } from 'svelte';
import SidebarSection from './SidebarSection.svelte';

function textChildren(text: string) {
	return createRawSnippet(() => ({
		render: () => `<span data-testid="content">${text}</span>`
	}));
}

describe('SidebarSection', () => {
	it('renders title and children when not collapsible', () => {
		render(SidebarSection, {
			props: {
				title: 'Contexts',
				children: textChildren('items')
			}
		});

		expect(screen.getByText('Contexts')).not.toBeNull();
		expect(screen.getByTestId('content')).not.toBeNull();
	});

	it('hides children when collapsible toggled closed', async () => {
		const { container } = render(SidebarSection, {
			props: {
				title: 'Labels',
				collapsible: true,
				defaultOpen: false,
				children: textChildren('items')
			}
		});

		expect(screen.queryByTestId('content')).toBeNull();
		const toggle = container.querySelector('button[aria-expanded]');
		expect(toggle).not.toBeNull();
		expect(toggle!.getAttribute('aria-expanded')).toBe('false');
	});
});
