import { describe, expect, it, vi } from 'vitest';
import { useListMutator } from './useListMutator.svelte';

interface Item {
	id: number;
	title: string;
}

describe('useListMutator', () => {
	it('starts with an empty list', () => {
		const list = useListMutator<Item>();
		expect(list.items).toEqual([]);
	});

	it('exposes a settable items array', () => {
		const list = useListMutator<Item>();
		list.items = [
			{ id: 1, title: 'a' },
			{ id: 2, title: 'b' }
		];
		expect(list.items.map((i) => i.id)).toEqual([1, 2]);
	});

	describe('replace', () => {
		it('updates the item with the matching id and leaves others alone', () => {
			const list = useListMutator<Item>();
			list.items = [
				{ id: 1, title: 'a' },
				{ id: 2, title: 'b' }
			];
			list.mutator.replace({ id: 2, title: 'B!' });
			expect(list.items).toEqual([
				{ id: 1, title: 'a' },
				{ id: 2, title: 'B!' }
			]);
		});

		it('is a no-op when the id is not present', () => {
			const list = useListMutator<Item>();
			list.items = [{ id: 1, title: 'a' }];
			list.mutator.replace({ id: 99, title: 'x' });
			expect(list.items).toEqual([{ id: 1, title: 'a' }]);
		});
	});

	describe('remove', () => {
		it('drops the item with the matching id', () => {
			const list = useListMutator<Item>();
			list.items = [
				{ id: 1, title: 'a' },
				{ id: 2, title: 'b' }
			];
			list.mutator.remove(1);
			expect(list.items.map((i) => i.id)).toEqual([2]);
		});

		it('invokes onRemove when configured', () => {
			const onRemove = vi.fn();
			const list = useListMutator<Item>({ onRemove });
			list.items = [{ id: 1, title: 'a' }];
			list.mutator.remove(1);
			expect(onRemove).toHaveBeenCalledTimes(1);
		});

		it('still calls onRemove when the id is not present', () => {
			// Current contract: remove always notifies; harmless and matches existing usage.
			const onRemove = vi.fn();
			const list = useListMutator<Item>({ onRemove });
			list.items = [{ id: 1, title: 'a' }];
			list.mutator.remove(42);
			expect(list.items).toHaveLength(1);
			expect(onRemove).toHaveBeenCalledTimes(1);
		});
	});

	describe('insertAfter', () => {
		it('inserts the item directly after the given id', () => {
			const list = useListMutator<Item>();
			list.items = [
				{ id: 1, title: 'a' },
				{ id: 2, title: 'b' },
				{ id: 3, title: 'c' }
			];
			list.mutator.insertAfter(2, { id: 99, title: 'new' });
			expect(list.items.map((i) => i.id)).toEqual([1, 2, 99, 3]);
		});

		it('appends to the end when the anchor id is missing', () => {
			const list = useListMutator<Item>();
			list.items = [{ id: 1, title: 'a' }];
			list.mutator.insertAfter(42, { id: 2, title: 'b' });
			expect(list.items.map((i) => i.id)).toEqual([1, 2]);
		});
	});

	describe('add', () => {
		it('appends the item', () => {
			const list = useListMutator<Item>();
			list.items = [{ id: 1, title: 'a' }];
			list.mutator.add({ id: 2, title: 'b' });
			expect(list.items.map((i) => i.id)).toEqual([1, 2]);
		});

		it('is idempotent on duplicate id', () => {
			const list = useListMutator<Item>();
			list.items = [{ id: 1, title: 'a' }];
			list.mutator.add({ id: 1, title: 'changed' });
			expect(list.items).toEqual([{ id: 1, title: 'a' }]);
		});
	});
});
