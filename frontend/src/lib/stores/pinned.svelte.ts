export interface PinnedTask {
	id: string;
	content: string;
}

const PINNED_KEY = 'turboist:pinned-tasks';

function loadPinned(): PinnedTask[] {
	try {
		const raw = localStorage.getItem(PINNED_KEY);
		return raw ? JSON.parse(raw) : [];
	} catch {
		return [];
	}
}

function createPinnedStore() {
	let items = $state<PinnedTask[]>(loadPinned());
	let maxPinned = $state(5);
	let _selectedTaskId = $state<string | null>(null);

	function save(): void {
		localStorage.setItem(PINNED_KEY, JSON.stringify(items));
	}

	function pin(task: PinnedTask): void {
		if (items.some((t) => t.id === task.id)) return;
		if (items.length >= maxPinned) return;
		items = [...items, task];
		save();
	}

	function unpin(taskId: string): void {
		items = items.filter((t) => t.id !== taskId);
		save();
	}

	function isPinned(taskId: string): boolean {
		return items.some((t) => t.id === taskId);
	}

	function setMaxPinned(value: number): void {
		maxPinned = value;
		if (items.length > maxPinned) {
			items = items.slice(0, maxPinned);
			save();
		}
	}

	function selectTask(taskId: string): void {
		_selectedTaskId = taskId;
	}

	function consumeSelection(): string | null {
		const id = _selectedTaskId;
		_selectedTaskId = null;
		return id;
	}

	return {
		get items() {
			return items;
		},
		get maxPinned() {
			return maxPinned;
		},
		get isFull() {
			return items.length >= maxPinned;
		},
		get selectedTaskId() {
			return _selectedTaskId;
		},
		pin,
		unpin,
		isPinned,
		setMaxPinned,
		selectTask,
		consumeSelection
	};
}

export const pinnedStore = createPinnedStore();
