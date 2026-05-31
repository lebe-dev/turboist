import { describe, it, expect } from 'vitest';
import { reconcileByVersion } from './reconcile';

type Row = { id: number; updatedAt: string; title: string };

const row = (id: number, updatedAt: string, title = `t${id}`): Row => ({ id, updatedAt, title });

describe('reconcileByVersion', () => {
	it('returns incoming when current is empty', () => {
		const incoming = [row(1, 'a')];
		expect(reconcileByVersion([], incoming)).toBe(incoming);
	});

	it('returns the same array reference when nothing changed', () => {
		const current = [row(1, 'a'), row(2, 'a')];
		const incoming = [row(1, 'a'), row(2, 'a')]; // fresh objects, same versions
		expect(reconcileByVersion(current, incoming)).toBe(current);
	});

	it('reuses unchanged object references and swaps only changed ones', () => {
		const current = [row(1, 'a'), row(2, 'a'), row(3, 'a')];
		const incoming = [row(1, 'a'), row(2, 'b', 'renamed'), row(3, 'a')];
		const result = reconcileByVersion(current, incoming);

		expect(result).not.toBe(current);
		expect(result[0]).toBe(current[0]); // unchanged → reused
		expect(result[2]).toBe(current[2]); // unchanged → reused
		expect(result[1]).toBe(incoming[1]); // version bumped → replaced
		expect(result[1].title).toBe('renamed');
	});

	it('follows incoming order and membership exactly', () => {
		const current = [row(1, 'a'), row(2, 'a')];
		const incoming = [row(2, 'a'), row(1, 'a'), row(3, 'a')]; // reordered + added
		const result = reconcileByVersion(current, incoming);

		expect(result.map((r) => r.id)).toEqual([2, 1, 3]);
		expect(result[0]).toBe(current[1]); // id 2 reused despite new position
		expect(result[1]).toBe(current[0]); // id 1 reused
		expect(result[2]).toBe(incoming[2]); // id 3 is new
	});

	it('drops entries absent from incoming', () => {
		const current = [row(1, 'a'), row(2, 'a')];
		const incoming = [row(1, 'a')];
		const result = reconcileByVersion(current, incoming);
		expect(result.map((r) => r.id)).toEqual([1]);
		expect(result[0]).toBe(current[0]);
	});
});
