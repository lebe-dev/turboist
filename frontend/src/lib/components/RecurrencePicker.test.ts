import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import { userEvent } from '@testing-library/user-event';
import RecurrencePicker from './RecurrencePicker.svelte';

vi.mock('svelte-intl-precompile', () => ({
	t: {
		subscribe(fn: (value: any) => void) {
			fn((key: string) => key);
			return () => {};
		}
	},
	locale: {
		subscribe(fn: (value: any) => void) {
			fn('en');
			return () => {};
		}
	}
}));

describe('RecurrencePicker', () => {
	const baseProps = {
		onSelect: vi.fn(),
		onRemove: vi.fn(),
		isRecurring: false,
	};

	beforeEach(() => {
		vi.clearAllMocks();
	});

	afterEach(() => {
		cleanup();
	});

	// --- Collapsed state ---

	it('renders collapsed by default with "none" label', () => {
		render(RecurrencePicker, { props: baseProps });
		expect(screen.getByText('task.recurrence.none')).toBeInTheDocument();
		expect(screen.queryByText('task.recurrence.everyDay')).not.toBeInTheDocument();
	});

	it('shows "active" label when isRecurring is true', () => {
		render(RecurrencePicker, { props: { ...baseProps, isRecurring: true } });
		expect(screen.getByText('task.recurrence.active')).toBeInTheDocument();
		expect(screen.queryByText('task.recurrence.none')).not.toBeInTheDocument();
	});

	it('has aria-expanded=false when collapsed', () => {
		render(RecurrencePicker, { props: baseProps });
		const trigger = screen.getByRole('button', { expanded: false });
		expect(trigger).toBeInTheDocument();
	});

	// --- Expanding/collapsing ---

	it('expands on click showing all presets', async () => {
		const user = userEvent.setup();
		render(RecurrencePicker, { props: baseProps });

		await user.click(screen.getByText('task.recurrence.none'));

		expect(screen.getByText('task.recurrence.everyDay')).toBeInTheDocument();
		expect(screen.getByText('task.recurrence.everyWeekday')).toBeInTheDocument();
		expect(screen.getByText('task.recurrence.custom')).toBeInTheDocument();
	});

	it('collapses on second click', async () => {
		const user = userEvent.setup();
		render(RecurrencePicker, { props: baseProps });

		const trigger = screen.getByText('task.recurrence.none');
		await user.click(trigger);
		expect(screen.getByText('task.recurrence.everyDay')).toBeInTheDocument();

		await user.click(trigger);
		expect(screen.queryByText('task.recurrence.everyDay')).not.toBeInTheDocument();
	});

	it('has aria-expanded=true when expanded', async () => {
		const user = userEvent.setup();
		render(RecurrencePicker, { props: baseProps });

		await user.click(screen.getByText('task.recurrence.none'));
		const trigger = screen.getByRole('button', { expanded: true });
		expect(trigger).toBeInTheDocument();
	});

	// --- Preset selection ---

	it('calls onSelect when a preset is clicked', async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(RecurrencePicker, { props: { ...baseProps, onSelect } });

		await user.click(screen.getByText('task.recurrence.none'));
		await user.click(screen.getByText('task.recurrence.everyDay'));

		expect(onSelect).toHaveBeenCalledOnce();
		expect(onSelect).toHaveBeenCalledWith('every day');
	});

	// --- Custom input ---

	it('shows custom input and submits on Enter', async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(RecurrencePicker, { props: { ...baseProps, onSelect } });

		await user.click(screen.getByText('task.recurrence.none'));
		await user.click(screen.getByText('task.recurrence.custom'));

		const input = screen.getByPlaceholderText('task.recurrence.customPlaceholder');
		await user.type(input, 'every 3 days{Enter}');

		expect(onSelect).toHaveBeenCalledOnce();
		expect(onSelect).toHaveBeenCalledWith('every 3 days');
	});

	it('hides custom input on Escape', async () => {
		const user = userEvent.setup();
		render(RecurrencePicker, { props: baseProps });

		await user.click(screen.getByText('task.recurrence.none'));
		await user.click(screen.getByText('task.recurrence.custom'));

		const input = screen.getByPlaceholderText('task.recurrence.customPlaceholder');
		await user.type(input, '{Escape}');

		expect(screen.queryByPlaceholderText('task.recurrence.customPlaceholder')).not.toBeInTheDocument();
		// Presets still visible
		expect(screen.getByText('task.recurrence.everyDay')).toBeInTheDocument();
	});

	// --- Remove recurrence ---

	it('does not show remove button when not recurring', async () => {
		const user = userEvent.setup();
		render(RecurrencePicker, { props: baseProps });

		await user.click(screen.getByText('task.recurrence.none'));
		expect(screen.queryByText('task.recurrence.remove')).not.toBeInTheDocument();
	});

	it('shows remove button when isRecurring and onRemove provided', async () => {
		const user = userEvent.setup();
		render(RecurrencePicker, { props: { ...baseProps, isRecurring: true } });

		await user.click(screen.getByText('task.recurrence.active'));
		expect(screen.getByText('task.recurrence.remove')).toBeInTheDocument();
	});

	it('calls onRemove when remove button clicked', async () => {
		const user = userEvent.setup();
		const onRemove = vi.fn();
		render(RecurrencePicker, { props: { ...baseProps, isRecurring: true, onRemove } });

		await user.click(screen.getByText('task.recurrence.active'));
		await user.click(screen.getByText('task.recurrence.remove'));

		expect(onRemove).toHaveBeenCalledOnce();
	});
});
