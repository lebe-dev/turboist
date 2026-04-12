import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import { userEvent } from '@testing-library/user-event';
import DailyConstraintsDialog from './DailyConstraintsDialog.svelte';
import { constraintsStore } from '$lib/stores/constraints.svelte';

vi.mock('svelte-intl-precompile', () => ({
	t: {
		subscribe(fn: (value: any) => void) {
			fn((key: string, opts?: any) => {
				if (opts?.values?.count !== undefined) return `${key}(${opts.values.count})`;
				return key;
			});
			return () => {};
		}
	}
}));

vi.mock('svelte-sonner', () => ({
	toast: { error: vi.fn(), success: vi.fn() }
}));

const mockRollDailyConstraints = vi.fn();
const mockSwapDailyConstraint = vi.fn();
const mockConfirmDailyConstraints = vi.fn();

vi.mock('$lib/api/client', () => ({
	rollDailyConstraints: (...args: unknown[]) => mockRollDailyConstraints(...args),
	swapDailyConstraint: (...args: unknown[]) => mockSwapDailyConstraint(...args),
	confirmDailyConstraints: (...args: unknown[]) => mockConfirmDailyConstraints(...args)
}));

function initConstraints(overrides: Partial<Parameters<typeof constraintsStore.updateDailyConstraints>[0]> = {}) {
	constraintsStore.init({
		enabled: true,
		label_blocks: [],
		day_part_caps: [],
		priority_floor: 4,
		postpone_budget: 0,
		postpone_budget_used: 0
	});
	constraintsStore.updateDailyConstraints({
		needs_selection: false,
		items: [],
		rerolls_used: 0,
		max_rerolls: 2,
		pool_size: 5,
		confirmed: false,
		...overrides
	});
}

describe('DailyConstraintsDialog', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	afterEach(() => {
		cleanup();
	});

	it('shows roll button on first roll (needs_selection, no items)', () => {
		initConstraints({ needs_selection: true, items: [], pool_size: 5 });
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.getByText('constraints.dailyRollAll')).toBeInTheDocument();
	});

	it('calls rollDailyConstraints when roll button clicked', async () => {
		initConstraints({ needs_selection: true, items: [], pool_size: 5 });
		mockRollDailyConstraints.mockResolvedValue({
			needs_selection: false,
			items: ['no sugar', 'no phone'],
			rerolls_used: 0,
			max_rerolls: 2,
			pool_size: 5,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		const user = userEvent.setup();
		await user.click(screen.getByText('constraints.dailyRollAll'));

		expect(mockRollDailyConstraints).toHaveBeenCalledOnce();
	});

	it('shows constraint items after roll', async () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar', 'no phone'],
			pool_size: 5
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.getByText('no sugar')).toBeInTheDocument();
		expect(screen.getByText('no phone')).toBeInTheDocument();
	});

	it('shows swap buttons when not confirmed and can reroll', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar', 'no phone'],
			rerolls_used: 0,
			max_rerolls: 2,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		const swapButtons = screen.getAllByTitle('constraints.dailySwap');
		expect(swapButtons).toHaveLength(2);
	});

	it('calls swapDailyConstraint with correct index when swap clicked', async () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar', 'no phone'],
			rerolls_used: 0,
			max_rerolls: 2,
			confirmed: false
		});
		mockSwapDailyConstraint.mockResolvedValue({
			needs_selection: false,
			items: ['no sugar', 'exercise'],
			rerolls_used: 1,
			max_rerolls: 2,
			pool_size: 5,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		const user = userEvent.setup();
		const swapButtons = screen.getAllByTitle('constraints.dailySwap');
		await user.click(swapButtons[1]);

		expect(mockSwapDailyConstraint).toHaveBeenCalledWith(1);
	});

	it('shows re-roll button when not confirmed and rerolls available', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			rerolls_used: 0,
			max_rerolls: 2,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.getByText('constraints.dailyReroll')).toBeInTheDocument();
	});

	it('calls rollDailyConstraints when re-roll button clicked', async () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			rerolls_used: 0,
			max_rerolls: 2,
			confirmed: false
		});
		mockRollDailyConstraints.mockResolvedValue({
			needs_selection: false,
			items: ['exercise'],
			rerolls_used: 1,
			max_rerolls: 2,
			pool_size: 5,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		const user = userEvent.setup();
		await user.click(screen.getByText('constraints.dailyReroll'));

		expect(mockRollDailyConstraints).toHaveBeenCalledOnce();
	});

	it('hides re-roll button when rerolls exhausted', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			rerolls_used: 2,
			max_rerolls: 2,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.queryByText('constraints.dailyReroll')).not.toBeInTheDocument();
	});

	it('hides swap buttons when rerolls exhausted', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar', 'no phone'],
			rerolls_used: 2,
			max_rerolls: 2,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.queryByTitle('constraints.dailySwap')).not.toBeInTheDocument();
	});

	it('shows confirm button when not confirmed', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.getByText('constraints.dailyConfirm')).toBeInTheDocument();
	});

	it('calls confirmDailyConstraints when confirm clicked', async () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			confirmed: false
		});
		mockConfirmDailyConstraints.mockResolvedValue({
			needs_selection: false,
			items: ['no sugar'],
			rerolls_used: 0,
			max_rerolls: 2,
			pool_size: 5,
			confirmed: true
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		const user = userEvent.setup();
		await user.click(screen.getByText('constraints.dailyConfirm'));

		expect(mockConfirmDailyConstraints).toHaveBeenCalledOnce();
	});

	it('shows OK button when confirmed', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			confirmed: true
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.getByText('OK')).toBeInTheDocument();
		expect(screen.queryByText('constraints.dailyConfirm')).not.toBeInTheDocument();
		expect(screen.queryByText('constraints.dailyReroll')).not.toBeInTheDocument();
	});

	it('does not render when open is false', () => {
		initConstraints({ items: ['no sugar'] });
		render(DailyConstraintsDialog, { props: { open: false } });

		expect(screen.queryByText('constraints.dailyTitle')).not.toBeInTheDocument();
	});

	it('shows rerolls remaining text', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			rerolls_used: 1,
			max_rerolls: 3,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.getByText('constraints.dailyRerollsRemaining(2)')).toBeInTheDocument();
	});

	it('shows no rerolls text when exhausted', () => {
		initConstraints({
			needs_selection: false,
			items: ['no sugar'],
			rerolls_used: 2,
			max_rerolls: 2,
			confirmed: false
		});
		render(DailyConstraintsDialog, { props: { open: true } });

		expect(screen.getByText('constraints.dailyNoRerolls')).toBeInTheDocument();
	});
});
