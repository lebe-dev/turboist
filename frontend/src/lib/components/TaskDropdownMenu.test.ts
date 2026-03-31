import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import { userEvent } from '@testing-library/user-event';
import TaskDropdownMenu from './TaskDropdownMenu.svelte';
import type { Task } from '$lib/api/types';

vi.mock('svelte-intl-precompile', () => ({
	t: {
		subscribe(fn: (value: any) => void) {
			fn((key: string) => key);
			return () => {};
		}
	}
}));

function makeTask(overrides: Partial<Task> = {}): Task {
	return {
		id: 'task-1',
		content: 'Test task',
		description: '',
		project_id: 'proj-1',
		section_id: null,
		parent_id: null,
		labels: [],
		priority: 1,
		due: null,
		sub_task_count: 0,
		completed_sub_task_count: 0,
		completed_at: null,
		added_at: '2026-01-01T00:00:00Z',
		is_project_task: false,
		postpone_count: 0,
		children: [],
		...overrides
	};
}

function todayStr(): string {
	const d = new Date();
	return (
		d.getFullYear() +
		'-' +
		String(d.getMonth() + 1).padStart(2, '0') +
		'-' +
		String(d.getDate()).padStart(2, '0')
	);
}

function tomorrowStr(): string {
	const d = new Date();
	d.setDate(d.getDate() + 1);
	return (
		d.getFullYear() +
		'-' +
		String(d.getMonth() + 1).padStart(2, '0') +
		'-' +
		String(d.getDate()).padStart(2, '0')
	);
}

describe('TaskDropdownMenu', () => {
	const baseProps = {
		open: true,
		task: makeTask(),
		onSetDate: vi.fn(),
		onSetPriority: vi.fn(),
		onDelete: vi.fn()
	};

	beforeEach(() => {
		vi.clearAllMocks();
	});

	afterEach(() => {
		cleanup();
	});

	// --- Conditional rendering ---

	it('renders delete button always', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.getByText('dialog.delete')).toBeInTheDocument();
	});

	it('renders edit item when onEdit provided', () => {
		render(TaskDropdownMenu, { props: { ...baseProps, onEdit: vi.fn() } });
		expect(screen.getByText('task.edit')).toBeInTheDocument();
	});

	it('does not render edit item when onEdit not provided', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.queryByText('task.edit')).not.toBeInTheDocument();
	});

	it('renders duplicate item when onDuplicate provided', () => {
		render(TaskDropdownMenu, { props: { ...baseProps, onDuplicate: vi.fn() } });
		expect(screen.getByText('task.duplicate')).toBeInTheDocument();
	});

	it('does not render duplicate item when onDuplicate not provided', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.queryByText('task.duplicate')).not.toBeInTheDocument();
	});

	it('renders copy item when onCopy provided', () => {
		render(TaskDropdownMenu, { props: { ...baseProps, onCopy: vi.fn() } });
		expect(screen.getByText('task.copy')).toBeInTheDocument();
	});

	it('does not render copy item when onCopy not provided', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.queryByText('task.copy')).not.toBeInTheDocument();
	});

	// --- Pin ---

	it('renders pin item when canPin and onPin provided', () => {
		render(TaskDropdownMenu, {
			props: { ...baseProps, canPin: true, onPin: vi.fn() }
		});
		expect(screen.getByText('task.pin')).toBeInTheDocument();
	});

	it('shows unpin label when isPinned is true', () => {
		render(TaskDropdownMenu, {
			props: { ...baseProps, canPin: true, isPinned: true, onPin: vi.fn() }
		});
		expect(screen.getByText('task.unpin')).toBeInTheDocument();
		expect(screen.queryByText('task.pin')).not.toBeInTheDocument();
	});

	it('does not render pin item when canPin is false', () => {
		render(TaskDropdownMenu, {
			props: { ...baseProps, canPin: false, onPin: vi.fn() }
		});
		expect(screen.queryByText('task.pin')).not.toBeInTheDocument();
		expect(screen.queryByText('task.unpin')).not.toBeInTheDocument();
	});

	// --- Backlog ---

	it('renders add-to-backlog item when backlogLabel and onToggleBacklog provided', () => {
		render(TaskDropdownMenu, {
			props: { ...baseProps, backlogLabel: 'backlog', onToggleBacklog: vi.fn() }
		});
		expect(screen.getByText('task.addToBacklog')).toBeInTheDocument();
	});

	it('shows remove-from-backlog when isInBacklog is true', () => {
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				backlogLabel: 'backlog',
				isInBacklog: true,
				onToggleBacklog: vi.fn()
			}
		});
		expect(screen.getByText('task.removeFromBacklog')).toBeInTheDocument();
		expect(screen.queryByText('task.addToBacklog')).not.toBeInTheDocument();
	});

	it('does not render backlog item when backlogLabel is empty', () => {
		render(TaskDropdownMenu, {
			props: { ...baseProps, backlogLabel: '', onToggleBacklog: vi.fn() }
		});
		expect(screen.queryByText('task.addToBacklog')).not.toBeInTheDocument();
	});

	// --- Bulk operations ---

	it('renders bulk operations when subtaskCount > 0 and handlers provided', () => {
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				subtaskCount: 3,
				onResetSubtaskPriorities: vi.fn(),
				onResetSubtaskLabels: vi.fn()
			}
		});
		expect(screen.getByText('task.bulkOperations')).toBeInTheDocument();
	});

	it('does not render bulk operations when subtaskCount is 0', () => {
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				subtaskCount: 0,
				onResetSubtaskPriorities: vi.fn(),
				onResetSubtaskLabels: vi.fn()
			}
		});
		expect(screen.queryByText('task.bulkOperations')).not.toBeInTheDocument();
	});

	// --- Date section ---

	it('renders today and tomorrow date buttons', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.getByLabelText('Today')).toBeInTheDocument();
		expect(screen.getByLabelText('Tomorrow')).toBeInTheDocument();
	});

	it('renders date picker button when onOpenDatePicker provided', () => {
		render(TaskDropdownMenu, {
			props: { ...baseProps, onOpenDatePicker: vi.fn() }
		});
		expect(screen.getByLabelText('Pick date')).toBeInTheDocument();
	});

	it('does not render date picker button when onOpenDatePicker not provided', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.queryByLabelText('Pick date')).not.toBeInTheDocument();
	});

	it('renders clear date button when task has due date and onClearDate provided', () => {
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				task: makeTask({ due: { date: '2026-03-18', recurring: false } }),
				onClearDate: vi.fn()
			}
		});
		expect(screen.getByLabelText('Clear date')).toBeInTheDocument();
	});

	it('does not render clear date button when task has no due date', () => {
		render(TaskDropdownMenu, {
			props: { ...baseProps, onClearDate: vi.fn() }
		});
		expect(screen.queryByLabelText('Clear date')).not.toBeInTheDocument();
	});

	// --- Priority section ---

	it('renders all four priority buttons', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.getByLabelText('P1')).toBeInTheDocument();
		expect(screen.getByLabelText('P2')).toBeInTheDocument();
		expect(screen.getByLabelText('P3')).toBeInTheDocument();
		expect(screen.getByLabelText('P4')).toBeInTheDocument();
	});

	// --- Date label sections ---

	it('renders Date and Priority section labels', () => {
		render(TaskDropdownMenu, { props: baseProps });
		expect(screen.getByText('task.date')).toBeInTheDocument();
		expect(screen.getByText('task.priority')).toBeInTheDocument();
	});

	// --- Callback invocations ---

	it('calls onEdit when edit item clicked', async () => {
		const user = userEvent.setup();
		const onEdit = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onEdit } });

		await user.click(screen.getByText('task.edit'));
		expect(onEdit).toHaveBeenCalledOnce();
	});

	it('calls onDuplicate when duplicate item clicked', async () => {
		const user = userEvent.setup();
		const onDuplicate = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onDuplicate } });

		await user.click(screen.getByText('task.duplicate'));
		expect(onDuplicate).toHaveBeenCalledOnce();
	});

	it('calls onCopy when copy item clicked', async () => {
		const user = userEvent.setup();
		const onCopy = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onCopy } });

		await user.click(screen.getByText('task.copy'));
		expect(onCopy).toHaveBeenCalledOnce();
	});

	it('calls onPin when pin item clicked', async () => {
		const user = userEvent.setup();
		const onPin = vi.fn();
		render(TaskDropdownMenu, {
			props: { ...baseProps, canPin: true, onPin }
		});

		await user.click(screen.getByText('task.pin'));
		expect(onPin).toHaveBeenCalledOnce();
	});

	it('calls onToggleBacklog when backlog item clicked', async () => {
		const user = userEvent.setup();
		const onToggleBacklog = vi.fn();
		render(TaskDropdownMenu, {
			props: { ...baseProps, backlogLabel: 'backlog', onToggleBacklog }
		});

		await user.click(screen.getByText('task.addToBacklog'));
		expect(onToggleBacklog).toHaveBeenCalledOnce();
	});

	it('calls onSetDate with today string when today button clicked', async () => {
		const user = userEvent.setup();
		const onSetDate = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onSetDate } });

		await user.click(screen.getByLabelText('Today'));
		expect(onSetDate).toHaveBeenCalledWith(todayStr());
	});

	it('calls onSetDate with tomorrow string when tomorrow button clicked', async () => {
		const user = userEvent.setup();
		const onSetDate = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onSetDate } });

		await user.click(screen.getByLabelText('Tomorrow'));
		expect(onSetDate).toHaveBeenCalledWith(tomorrowStr());
	});

	it('calls onOpenDatePicker when pick date button clicked', async () => {
		const user = userEvent.setup();
		const onOpenDatePicker = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onOpenDatePicker } });

		await user.click(screen.getByLabelText('Pick date'));
		expect(onOpenDatePicker).toHaveBeenCalledOnce();
	});

	it('calls onClearDate when clear date button clicked', async () => {
		const user = userEvent.setup();
		const onClearDate = vi.fn();
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				task: makeTask({ due: { date: '2026-03-18', recurring: false } }),
				onClearDate
			}
		});

		await user.click(screen.getByLabelText('Clear date'));
		expect(onClearDate).toHaveBeenCalledOnce();
	});

	it.each([
		{ label: 'P1', value: 4 },
		{ label: 'P2', value: 3 },
		{ label: 'P3', value: 2 },
		{ label: 'P4', value: 1 }
	])('calls onSetPriority($value) when $label clicked', async ({ label, value }) => {
		const user = userEvent.setup();
		const onSetPriority = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onSetPriority } });

		await user.click(screen.getByLabelText(label));
		expect(onSetPriority).toHaveBeenCalledWith(value);
		cleanup();
	});

	it('calls onDelete when delete item clicked', async () => {
		const user = userEvent.setup();
		const onDelete = vi.fn();
		render(TaskDropdownMenu, { props: { ...baseProps, onDelete } });

		await user.click(screen.getByText('dialog.delete'));
		expect(onDelete).toHaveBeenCalledOnce();
	});

	// --- Active state highlighting ---

	it('today button has active style when task due date is today', () => {
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				task: makeTask({ due: { date: todayStr(), recurring: false } })
			}
		});
		const btn = screen.getByLabelText('Today');
		expect(btn.className).toContain('bg-accent');
	});

	it('tomorrow button has active style when task due date is tomorrow', () => {
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				task: makeTask({ due: { date: tomorrowStr(), recurring: false } })
			}
		});
		const btn = screen.getByLabelText('Tomorrow');
		expect(btn.className).toContain('bg-accent');
	});

	it('priority button has active style when matching task priority', () => {
		render(TaskDropdownMenu, {
			props: {
				...baseProps,
				task: makeTask({ priority: 4 })
			}
		});
		const p1Btn = screen.getByLabelText('P1');
		expect(p1Btn.className).toContain('bg-accent');

		const p2Btn = screen.getByLabelText('P2');
		// Active class is 'bg-accent' (no prefix), inactive is 'hover:bg-accent'
		expect(p2Btn.className).toMatch(/hover:bg-accent/);
		expect(p2Btn.className).not.toMatch(/(?<![:\w-])bg-accent/);
	});
});
