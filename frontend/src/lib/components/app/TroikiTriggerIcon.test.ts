import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import TroikiTriggerIcon from './TroikiTriggerIcon.svelte';

describe('TroikiTriggerIcon', () => {
	it('renders an svg with three circles forming the triangle of dots', () => {
		const { container } = render(TroikiTriggerIcon);
		const svg = container.querySelector('svg');
		expect(svg).not.toBeNull();
		expect(svg!.getAttribute('aria-hidden')).toBe('true');
		expect(svg!.querySelectorAll('circle').length).toBe(3);
	});

	it('forwards the class prop onto the svg element', () => {
		const { container } = render(TroikiTriggerIcon, { props: { class: 'h-5 w-5 text-muted' } });
		const svg = container.querySelector('svg')!;
		expect(svg.getAttribute('class')).toBe('h-5 w-5 text-muted');
	});

	it('omits the class attribute when none is supplied', () => {
		const { container } = render(TroikiTriggerIcon);
		const svg = container.querySelector('svg')!;
		// Empty default → class is either absent or empty; either way no styles applied.
		const cls = svg.getAttribute('class') ?? '';
		expect(cls).toBe('');
	});
});
