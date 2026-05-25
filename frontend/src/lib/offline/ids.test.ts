import { describe, it, expect } from 'vitest';
import { newClientId, newOutboxId, ref, isRef } from './ids';

describe('ids', () => {
	it('newClientId returns a 26-char ulid-like string', () => {
		const id = newClientId();
		expect(typeof id).toBe('string');
		expect(id).toHaveLength(26);
	});

	it('newClientId yields unique values', () => {
		const ids = new Set(Array.from({ length: 100 }, () => newClientId()));
		expect(ids.size).toBe(100);
	});

	it('newOutboxId yields a ulid string', () => {
		const id = newOutboxId();
		expect(id).toHaveLength(26);
	});

	it('ref creates a tagged reference', () => {
		const r = ref('abc');
		expect(r).toEqual({ kind: 'ref', clientId: 'abc' });
	});

	it('isRef recognizes refs and rejects non-refs', () => {
		expect(isRef(ref('x'))).toBe(true);
		expect(isRef({ clientId: 'x' })).toBe(false);
		expect(isRef(null)).toBe(false);
		expect(isRef('x')).toBe(false);
		expect(isRef({ kind: 'ref', clientId: 123 })).toBe(false);
	});
});
